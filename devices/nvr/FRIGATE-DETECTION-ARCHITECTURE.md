# Frigate Object Detection Pipeline Architecture

This document describes the internal architecture of Frigate's object detection pipeline - how video streams flow from cameras through detection to events.

## Overview

Frigate uses a **multi-process, distributed pipeline** with:
- **Shared memory** for efficient inter-process frame/tensor transfer
- **ZMQ pub/sub messaging** for signaling between processes
- **Region-based detection** to minimize inference load

```
┌──────────────┐    ┌──────────────┐    ┌──────────────┐    ┌──────────────┐
│   Camera     │───▶│   Capture    │───▶│  Processing  │───▶│    Event     │
│  (RTSP)      │    │   Process    │    │   Process    │    │  Processing  │
└──────────────┘    └──────────────┘    └──────────────┘    └──────────────┘
                           │                   │
                           ▼                   ▼
                    ┌──────────────┐    ┌──────────────┐
                    │    Shared    │    │   Detector   │
                    │    Memory    │◀──▶│   Process    │
                    └──────────────┘    └──────────────┘
```

## Process Architecture

| Process | Name | Priority | Purpose |
|---------|------|----------|---------|
| Capture | `frigate.capture:{camera}` | High | FFmpeg decode, frame ingestion |
| Processing | `frigate.process:{camera}` | High | Motion, detection, tracking |
| Detector | `frigate.detector:{name}` | Normal | Inference (shared across cameras) |
| Events | `frigate.events` | Normal | Event persistence, clips |
| Recording | `frigate.recording` | Normal | Segment management |

## Complete Pipeline Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 1. VIDEO CAPTURE                                                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  RTSP Stream (IP Camera)                                                   │
│         │                                                                   │
│         ▼                                                                   │
│  FFmpeg Subprocess (network_mode: host)                                    │
│  • Connects to camera RTSP URL                                             │
│  • Decodes H.264/H.265 to raw frames                                       │
│  • Outputs YUV 4:2:0 format via stdout                                     │
│         │                                                                   │
│         ▼                                                                   │
│  CameraCapture Process                                                     │
│  • Reads frames from FFmpeg stdout                                         │
│  • Writes to pre-allocated SharedMemory buffers                           │
│  • Rotating buffer pool (~20 frames per camera)                           │
│  • Frame size = height * width * 1.5 (YUV 4:2:0)                          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
                                  │
                          Frame Queue (SHM)
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ 2. MOTION DETECTION                                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ImprovedMotionDetector                                                    │
│         │                                                                   │
│         ├─▶ Extract grayscale from YUV frame                              │
│         ├─▶ Resize to motion detection size (smaller = faster)            │
│         ├─▶ Apply contrast improvement (percentile clipping)              │
│         │                                                                   │
│         ▼                                                                   │
│  Frame Differencing                                                        │
│  • Maintain running average frame (avg_frame)                              │
│  • Maintain running average delta (avg_delta)                              │
│  • delta = |current_frame - avg_frame|                                     │
│  • avg_delta = (1-alpha)*avg_delta + alpha*delta                          │
│         │                                                                   │
│         ▼                                                                   │
│  Threshold & Contour Detection                                             │
│  • Threshold: pixels where delta > threshold                               │
│  • Dilate to fill holes                                                    │
│  • Find contours                                                           │
│  • Filter by minimum contour area                                          │
│         │                                                                   │
│         ▼                                                                   │
│  Output: motion_boxes (bounding boxes of motion regions)                   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ 3. REGION SELECTION (Smart Detection Optimization)                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Why? Full-frame detection every frame is expensive.                       │
│  Solution: Only detect in regions likely to contain objects.               │
│                                                                             │
│  Region Sources:                                                           │
│  ┌─────────────────────────────────────────────────────────────┐          │
│  │ 1. Tracked Object Regions                                    │          │
│  │    • Box around each currently tracked object               │          │
│  │    • Use predicted position for moving objects              │          │
│  │    • Use last position for stationary objects               │          │
│  └─────────────────────────────────────────────────────────────┘          │
│  ┌─────────────────────────────────────────────────────────────┐          │
│  │ 2. Motion Box Regions                                        │          │
│  │    • Convert motion_boxes to detection regions              │          │
│  │    • Skip regions already covered by tracked objects        │          │
│  └─────────────────────────────────────────────────────────────┘          │
│  ┌─────────────────────────────────────────────────────────────┐          │
│  │ 3. Stationary Object Re-detection                           │          │
│  │    • Objects that haven't moved in N frames                 │          │
│  │    • Re-detect periodically (stationary.interval)           │          │
│  └─────────────────────────────────────────────────────────────┘          │
│  ┌─────────────────────────────────────────────────────────────┐          │
│  │ 4. Startup Scan                                              │          │
│  │    • First frame: full grid detection                       │          │
│  │    • Finds objects not initially moving                     │          │
│  └─────────────────────────────────────────────────────────────┘          │
│                                                                             │
│  Output: regions (list of detection regions to process)                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ 4. OBJECT DETECTION (Per Region)                                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  For each region in regions:                                               │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────┐          │
│  │ 4a. Tensor Creation (RemoteObjectDetector)                  │          │
│  │     • Extract region from YUV frame                         │          │
│  │     • Convert YUV → RGB/BGR                                 │          │
│  │     • Resize to model input (default 320x320)               │          │
│  │     • Normalize if required (float/denorm)                  │          │
│  │     • Write tensor to SharedMemory: {camera_name}           │          │
│  └─────────────────────────────────────────────────────────────┘          │
│         │                                                                   │
│         │ Put camera_name on detection_queue                               │
│         ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────┐          │
│  │ 4b. Detector Process (DetectorRunner)                       │          │
│  │     • Receive camera_name from detection_queue              │          │
│  │     • Read tensor from SharedMemory: {camera_name}          │          │
│  │     • Run inference via backend (see below)                 │          │
│  │     • Write results to SharedMemory: out-{camera_name}      │          │
│  │     • Publish completion via ZMQ                            │          │
│  │     • Measure inference time                                │          │
│  └─────────────────────────────────────────────────────────────┘          │
│         │                                                                   │
│         │ ZMQ notification                                                 │
│         ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────┐          │
│  │ 4c. Read Results (RemoteObjectDetector)                     │          │
│  │     • Wait for ZMQ completion signal                        │          │
│  │     • Read from SharedMemory: out-{camera_name}             │          │
│  │     • 20 detections max, 6 values each:                     │          │
│  │       [label_id, confidence, x1_norm, y1_norm, x2_norm, y2_norm]       │
│  │     • Convert normalized coords → frame coords              │          │
│  └─────────────────────────────────────────────────────────────┘          │
│                                                                             │
│  Supported Detector Backends:                                              │
│  ┌────────────────┬─────────────────────────────────────────────┐         │
│  │ Backend        │ Use Case                                     │         │
│  ├────────────────┼─────────────────────────────────────────────┤         │
│  │ TensorFlow Lite│ CPU inference, EdgeTPU (Coral)              │         │
│  │ ONNX Runtime   │ Generic, cross-platform                      │         │
│  │ TensorRT       │ NVIDIA GPU (fast)                            │         │
│  │ OpenVINO       │ Intel CPU/GPU, ARM                           │         │
│  │ RKNN           │ Rockchip NPU (RK3588, etc)                  │         │
│  │ Hailo8L        │ Hailo AI accelerator                         │         │
│  │ MemryX         │ MemryX accelerator (async)                   │         │
│  └────────────────┴─────────────────────────────────────────────┘         │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ 5. DETECTION CONSOLIDATION & FILTERING                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Deduplication (reduce_detections)                                         │
│  • Overlapping regions may detect same object                              │
│  • Keep detection with highest confidence                                  │
│  • Merge overlapping bounding boxes                                        │
│                                                                             │
│  Object Filtering                                                          │
│  • Confidence threshold (skip low-confidence)                              │
│  • Min/max area (skip too small or too large)                              │
│  • Labels filter (only track configured objects)                           │
│  • Zone filters (only track in monitored zones)                            │
│  • Mask filters (exclude static regions)                                   │
│                                                                             │
│  Output: consolidated_detections                                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ 6. OBJECT TRACKING                                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  NorfairTracker                                                            │
│                                                                             │
│  Purpose: Maintain persistent object identities across frames              │
│                                                                             │
│  Algorithm:                                                                │
│  1. Match detections to existing tracks                                    │
│     • Calculate distance from each detection to tracked objects            │
│     • Hungarian algorithm for optimal assignment                           │
│     • Only match if distance < threshold                                   │
│                                                                             │
│  2. Update track estimates                                                 │
│     • Kalman filter predicts next position                                 │
│     • Update velocity estimates                                            │
│                                                                             │
│  3. Track management                                                       │
│     • Disappearance counter: frames since last detection                   │
│     • Motionless counter: frames without movement                          │
│     • Remove tracks after max_disappeared frames                           │
│                                                                             │
│  TrackedObject State:                                                      │
│  ┌─────────────────────────────────────────────────────────────┐          │
│  │ • obj_data: core detection data                              │          │
│  │ • id: persistent tracking ID                                 │          │
│  │ • score_history: confidence over time                        │          │
│  │ • zone_presence: time spent in each zone                     │          │
│  │ • zone_loitering: loitering detection                        │          │
│  │ • attributes: classified attributes (color, etc)             │          │
│  │ • top_score: best confidence seen                            │          │
│  │ • speed_history: velocity measurements                       │          │
│  │ • has_clip / has_snapshot: media status                      │          │
│  └─────────────────────────────────────────────────────────────┘          │
│                                                                             │
│  Output: tracked_objects (dict with persistent IDs)                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ 7. EVENT GENERATION                                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Queue to detected_objects_queue:                                          │
│  (camera_name, frame_name, frame_time, tracked_objects, motion_boxes, ...)│
│                                                                             │
│  TrackedObjectProcessor (separate thread)                                  │
│  • Updates object state (zones, attributes, loitering)                     │
│  • Filters false positives                                                 │
│  • Publishes events via ZMQ EventUpdatePublisher                          │
│                                                                             │
│  Event States: start → update* → end                                       │
│                                                                             │
│  EventProcessor (threading.Thread)                                         │
│  • Receives events via ZMQ EventUpdateSubscriber                          │
│  • Stores in events_in_process dict                                        │
│  • Updates database on state change                                        │
│  • Triggers on event end:                                                  │
│    - Snapshot generation (best frame + thumbnail)                          │
│    - Clip extraction (from recording segments)                             │
│    - Notifications (MQTT, webhooks)                                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ 8. RECORDING & STORAGE                                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Recording Process (parallel to detection)                                 │
│  • Separate FFmpeg process streams segments                                │
│  • Writes to RECORD_DIR (/media/frigate/recordings/)                      │
│  • Organized: {camera}/{year}/{month}/{day}/{hour}/                       │
│  • Segments tracked in database                                            │
│                                                                             │
│  Clip Generation (on event end)                                            │
│  • Clips extracted from recording segments                                 │
│  • Stored in CLIPS_DIR                                                     │
│  • Pre-capture and post-capture padding                                    │
│                                                                             │
│  Retention                                                                 │
│  • Configurable per-camera retention days                                  │
│  • Mode: all, motion, or active_objects                                    │
│  • Automatic cleanup of old segments                                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Inter-Process Communication (IPC)

### Shared Memory
| Buffer | Purpose | Format |
|--------|---------|--------|
| Frame buffers | Raw video frames | YUV 4:2:0, rotating pool |
| `{camera_name}` | Tensor input | Model input size (320x320x3) |
| `out-{camera_name}` | Detection results | 20 detections x 6 values |

### Message Queues
| Queue | Purpose |
|-------|---------|
| `detection_queue` | Camera names signaling detector |
| `detected_frames_queue` | Processed frames with objects |
| `timeline_queue` | Timeline/database updates |
| `log_queue` | Centralized logging |

### ZMQ Pub/Sub
| Channel | Purpose |
|---------|---------|
| ObjectDetector | Detector completion signals |
| EventUpdate | Event state changes |
| CameraStatus | Camera health updates |

## Key Configuration Parameters

### Motion Detection (`motion:`)
```yaml
motion:
  threshold: 30          # Pixel delta sensitivity (1-255)
  contour_area: 10       # Min motion region size
  delta_alpha: 0.2       # Motion averaging weight
  frame_alpha: 0.01      # Baseline frame averaging
  frame_height: 100      # Detection frame height (lower = faster)
  mask: 0,0,200,200      # Exclusion polygon
```

### Object Detection (`detect:`)
```yaml
detect:
  enabled: true
  width: 1920            # Input frame width
  height: 1080           # Input frame height
  fps: 5                 # Target detection FPS
  stationary:
    threshold: 50        # Frames before considered stationary
    interval: 50         # Re-detection frequency (frames)
```

### Detector Backend (`detectors:`)
```yaml
detectors:
  coral:
    type: edgetpu        # cpu, tensorrt, openvino, onnx, edgetpu, hailo, rknn
    device: usb          # Device path or identifier
```

## Performance Characteristics

### Typical Latency (2MP @ 10fps, GPU detection)
| Stage | Time |
|-------|------|
| Capture | ~0ms (passive) |
| Motion detection | 5-10ms |
| Region selection | 2-5ms |
| Tensor creation | 2-5ms |
| Detection (GPU) | 20-50ms |
| Tracking | 5-10ms |
| **Total** | **35-80ms** |

With 10fps input (100ms between frames), the pipeline typically keeps up or skips frames gracefully.

### Optimizations
1. **Region-based detection** - Only detect where motion/objects exist
2. **Multi-process** - Parallel capture, processing, detection
3. **Shared memory** - Zero-copy frame transfer
4. **Hardware acceleration** - Offload to TPU/GPU/NPU
5. **Frame skipping** - Detect every Nth frame under load
6. **Stationary handling** - Reduce redundant re-detections

## Key Source Files

| Component | File |
|-----------|------|
| Main app | `frigate/app.py` |
| Video capture | `frigate/video.py` |
| Motion detection | `frigate/motion/frigate_motion.py` |
| Object detection | `frigate/object_detection/base.py` |
| Detector backends | `frigate/detectors/plugins/*.py` |
| Object tracking | `frigate/track/norfair_tracker.py` |
| Event processing | `frigate/events/maintainer.py` |
| Frame manager | `frigate/util/image.py` |

## References

- [Frigate Documentation](https://docs.frigate.video/)
- [Frigate GitHub](https://github.com/blakeblackshear/frigate)
- [Norfair Tracker](https://github.com/tryolabs/norfair)
