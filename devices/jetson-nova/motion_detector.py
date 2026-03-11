#!/usr/bin/env python3
"""
Motion Detection WebSocket Server for Jetson Nano

Captures video from USB camera, detects motion using frame differencing,
and broadcasts events via WebSocket in a format compatible with the
dashboard simulator protocol.

Usage:
    python3 motion_detector.py [--port 8081] [--camera 0] [--threshold 25]
"""

import asyncio
import argparse
import json
import time
import logging
from datetime import datetime
from typing import Set
import signal
import sys

import cv2
import numpy as np

# WebSocket imports - will use websockets library
try:
    import websockets
except ImportError:
    print("websockets library not found. Install with: pip3 install websockets")
    sys.exit(1)

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class MotionDetector:
    """
    Motion detector using background subtraction.
    Can be replaced with a more sophisticated detector (e.g., face recognition)
    by implementing the same interface.
    """

    def __init__(self, threshold: int = 25, min_area: int = 500):
        """
        Initialize motion detector.

        Args:
            threshold: Pixel difference threshold for motion detection
            min_area: Minimum contour area to consider as motion
        """
        self.threshold = threshold
        self.min_area = min_area
        self.prev_frame = None
        self.bg_subtractor = cv2.createBackgroundSubtractorMOG2(
            history=500,
            varThreshold=threshold,
            detectShadows=True
        )

    def detect(self, frame: np.ndarray) -> dict:
        """
        Detect motion in the given frame.

        Args:
            frame: BGR image from camera

        Returns:
            dict with detection results:
                - motion_detected: bool
                - motion_level: float (0-100)
                - regions: list of bounding boxes
                - frame_processed: processed frame for debugging
        """
        # Convert to grayscale
        gray = cv2.cvtColor(frame, cv2.COLOR_BGR2GRAY)
        gray = cv2.GaussianBlur(gray, (21, 21), 0)

        # Apply background subtraction
        fg_mask = self.bg_subtractor.apply(frame)

        # Remove shadows (marked as 127 in MOG2)
        fg_mask = cv2.threshold(fg_mask, 250, 255, cv2.THRESH_BINARY)[1]

        # Dilate to fill gaps
        fg_mask = cv2.dilate(fg_mask, None, iterations=2)

        # Find contours
        contours, _ = cv2.findContours(
            fg_mask.copy(),
            cv2.RETR_EXTERNAL,
            cv2.CHAIN_APPROX_SIMPLE
        )

        motion_detected = False
        regions = []
        total_motion_area = 0
        frame_area = frame.shape[0] * frame.shape[1]

        for contour in contours:
            area = cv2.contourArea(contour)
            if area < self.min_area:
                continue

            motion_detected = True
            total_motion_area += area
            x, y, w, h = cv2.boundingRect(contour)
            regions.append({'x': x, 'y': y, 'width': w, 'height': h})

        # Calculate motion level as percentage of frame with motion
        motion_level = min(100.0, (total_motion_area / frame_area) * 100 * 10)

        return {
            'motion_detected': motion_detected,
            'motion_level': round(motion_level, 2),
            'regions': regions,
            'contour_count': len(regions)
        }


class CameraCapture:
    """Handles camera capture with proper resource management."""

    def __init__(self, camera_id: int = 0, width: int = 640, height: int = 480):
        self.camera_id = camera_id
        self.width = width
        self.height = height
        self.cap = None

    def open(self) -> bool:
        """Open the camera."""
        self.cap = cv2.VideoCapture(self.camera_id)
        if not self.cap.isOpened():
            logger.error(f"Failed to open camera {self.camera_id}")
            return False

        self.cap.set(cv2.CAP_PROP_FRAME_WIDTH, self.width)
        self.cap.set(cv2.CAP_PROP_FRAME_HEIGHT, self.height)
        self.cap.set(cv2.CAP_PROP_FPS, 30)

        # Read actual settings
        actual_width = self.cap.get(cv2.CAP_PROP_FRAME_WIDTH)
        actual_height = self.cap.get(cv2.CAP_PROP_FRAME_HEIGHT)
        actual_fps = self.cap.get(cv2.CAP_PROP_FPS)

        logger.info(f"Camera opened: {actual_width}x{actual_height} @ {actual_fps}fps")
        return True

    def read(self) -> tuple:
        """Read a frame from the camera."""
        if self.cap is None:
            return False, None
        return self.cap.read()

    def close(self):
        """Release camera resources."""
        if self.cap is not None:
            self.cap.release()
            self.cap = None
            logger.info("Camera released")


class MotionWebSocketServer:
    """
    WebSocket server that broadcasts motion detection events.

    Event format matches the dashboard simulator protocol:
    {
        "timestamp": 1701234567890,
        "sensor_id": "camera-001",
        "sensor_type": "motion",
        "value": 1.0,
        "unit": "boolean",
        "location": "jetson-nano",
        "status": "normal",
        "quality": 95
    }
    """

    def __init__(
        self,
        host: str = "0.0.0.0",
        port: int = 8081,
        camera_id: int = 0,
        sensor_id: str = "camera-001",
        location: str = "jetson-nano",
        detection_threshold: int = 25,
        min_motion_area: int = 500,
        cooldown_seconds: float = 1.0
    ):
        self.host = host
        self.port = port
        self.camera_id = camera_id
        self.sensor_id = sensor_id
        self.location = location
        self.detection_threshold = detection_threshold
        self.min_motion_area = min_motion_area
        self.cooldown_seconds = cooldown_seconds

        self.clients: Set[websockets.WebSocketServerProtocol] = set()
        self.camera = CameraCapture(camera_id)
        self.detector = MotionDetector(
            threshold=detection_threshold,
            min_area=min_motion_area
        )

        self.running = False
        self.last_motion_time = 0
        self.stats = {
            'frames_processed': 0,
            'motion_events': 0,
            'start_time': None
        }

    async def register(self, websocket: websockets.WebSocketServerProtocol):
        """Register a new WebSocket client."""
        self.clients.add(websocket)
        logger.info(f"Client connected. Total clients: {len(self.clients)}")

    async def unregister(self, websocket: websockets.WebSocketServerProtocol):
        """Unregister a WebSocket client."""
        self.clients.discard(websocket)
        logger.info(f"Client disconnected. Total clients: {len(self.clients)}")

    async def broadcast(self, message: dict):
        """Broadcast a message to all connected clients."""
        if not self.clients:
            return

        data = json.dumps(message)
        disconnected = set()

        for client in self.clients:
            try:
                await client.send(data)
            except websockets.ConnectionClosed:
                disconnected.add(client)
            except Exception as e:
                logger.error(f"Error sending to client: {e}")
                disconnected.add(client)

        # Clean up disconnected clients
        for client in disconnected:
            self.clients.discard(client)

    def create_event(self, detection_result: dict) -> dict:
        """Create an event message in the dashboard protocol format."""
        motion_detected = detection_result['motion_detected']
        motion_level = detection_result['motion_level']

        # Determine status based on motion level
        if motion_level > 50:
            status = "warning"
        elif motion_level > 80:
            status = "error"
        else:
            status = "normal"

        return {
            "timestamp": int(time.time() * 1000),
            "sensor_id": self.sensor_id,
            "sensor_type": "motion",
            "value": motion_level if motion_detected else 0.0,
            "unit": "percent",
            "location": self.location,
            "status": status,
            "quality": 95,
            "metadata": {
                "motion_detected": motion_detected,
                "regions_count": detection_result['contour_count'],
                "regions": detection_result['regions'][:5]  # Limit to 5 regions
            }
        }

    async def handle_client(self, websocket: websockets.WebSocketServerProtocol, path: str):
        """Handle a WebSocket client connection."""
        await self.register(websocket)
        try:
            # Handle incoming messages (for configuration updates)
            async for message in websocket:
                try:
                    cmd = json.loads(message)
                    if cmd.get('command') == 'get_stats':
                        await websocket.send(json.dumps({
                            'type': 'stats',
                            'data': self.stats
                        }))
                    elif cmd.get('command') == 'set_threshold':
                        new_threshold = cmd.get('threshold', self.detection_threshold)
                        self.detector.threshold = new_threshold
                        logger.info(f"Threshold updated to {new_threshold}")
                except json.JSONDecodeError:
                    pass
        except websockets.ConnectionClosed:
            pass
        finally:
            await self.unregister(websocket)

    async def detection_loop(self):
        """Main detection loop - processes frames and broadcasts events."""
        logger.info("Starting detection loop...")

        while self.running:
            ret, frame = self.camera.read()
            if not ret:
                logger.warning("Failed to read frame")
                await asyncio.sleep(0.1)
                continue

            self.stats['frames_processed'] += 1

            # Run detection
            result = self.detector.detect(frame)

            # Apply cooldown to avoid flooding with events
            current_time = time.time()
            if result['motion_detected']:
                if current_time - self.last_motion_time >= self.cooldown_seconds:
                    self.last_motion_time = current_time
                    self.stats['motion_events'] += 1

                    event = self.create_event(result)
                    await self.broadcast(event)
                    logger.debug(f"Motion detected: level={result['motion_level']}")

            # Small delay to control frame rate
            await asyncio.sleep(0.033)  # ~30 FPS

    async def run(self):
        """Start the WebSocket server and detection loop."""
        # Open camera
        if not self.camera.open():
            raise RuntimeError("Failed to open camera")

        self.running = True
        self.stats['start_time'] = datetime.now().isoformat()

        # Start WebSocket server
        server = await websockets.serve(
            self.handle_client,
            self.host,
            self.port
        )

        logger.info(f"WebSocket server started on ws://{self.host}:{self.port}")
        logger.info(f"Sensor ID: {self.sensor_id}, Location: {self.location}")

        # Run detection loop
        try:
            await self.detection_loop()
        finally:
            self.running = False
            self.camera.close()
            server.close()
            await server.wait_closed()

    def stop(self):
        """Signal the server to stop."""
        self.running = False
        logger.info("Shutdown requested")


def main():
    parser = argparse.ArgumentParser(description='Motion Detection WebSocket Server')
    parser.add_argument('--host', default='0.0.0.0', help='Server host (default: 0.0.0.0)')
    parser.add_argument('--port', type=int, default=8081, help='Server port (default: 8081)')
    parser.add_argument('--camera', type=int, default=0, help='Camera device ID (default: 0)')
    parser.add_argument('--sensor-id', default='camera-001', help='Sensor ID for events')
    parser.add_argument('--location', default='jetson-nano', help='Location identifier')
    parser.add_argument('--threshold', type=int, default=25, help='Motion detection threshold')
    parser.add_argument('--min-area', type=int, default=500, help='Minimum motion area in pixels')
    parser.add_argument('--cooldown', type=float, default=30.0, help='Seconds between motion events')
    parser.add_argument('--debug', action='store_true', help='Enable debug logging')

    args = parser.parse_args()

    if args.debug:
        logging.getLogger().setLevel(logging.DEBUG)

    server = MotionWebSocketServer(
        host=args.host,
        port=args.port,
        camera_id=args.camera,
        sensor_id=args.sensor_id,
        location=args.location,
        detection_threshold=args.threshold,
        min_motion_area=args.min_area,
        cooldown_seconds=args.cooldown
    )

    # Handle shutdown signals
    def signal_handler(sig, frame):
        logger.info("Received shutdown signal")
        server.stop()

    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    # Run the server
    try:
        asyncio.get_event_loop().run_until_complete(server.run())
    except KeyboardInterrupt:
        logger.info("Interrupted by user")
    except Exception as e:
        logger.error(f"Server error: {e}")
        raise


if __name__ == '__main__':
    main()
