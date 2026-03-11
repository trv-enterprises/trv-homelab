/**
 * Zigbee2MQTT codec for homebridge-mqttthing
 *
 * Handles JSON payload extraction and conversion for:
 *   - Hue color bulbs (XY→HS conversion, brightness scaling)
 *   - Shelly plugs (state extraction from JSON)
 *   - Soil moisture sensors (moisture, battery)
 *   - Contact sensors / garage door (contact state)
 */

'use strict';

function init(params) {
  const { log, config, publish, notify } = params;

  // ── XY ↔ HS color conversion ──────────────────────────────────
  // CIE 1931 XY to Hue/Saturation (simplified, good enough for Hue bulbs)
  function xyToHs(x, y) {
    // Convert XY to RGB first
    const z = 1.0 - x - y;
    const Y = 1.0; // brightness normalized
    const X = (Y / y) * x;
    const Z = (Y / y) * z;

    let r =  X * 1.656492 - Y * 0.354851 - Z * 0.255038;
    let g = -X * 0.707196 + Y * 1.655397 + Z * 0.036152;
    let b =  X * 0.051713 - Y * 0.121364 + Z * 1.011530;

    // Clamp and gamma correct
    r = Math.max(0, Math.min(1, r));
    g = Math.max(0, Math.min(1, g));
    b = Math.max(0, Math.min(1, b));

    r = r <= 0.0031308 ? 12.92 * r : (1.055 * Math.pow(r, 1.0 / 2.4) - 0.055);
    g = g <= 0.0031308 ? 12.92 * g : (1.055 * Math.pow(g, 1.0 / 2.4) - 0.055);
    b = b <= 0.0031308 ? 12.92 * b : (1.055 * Math.pow(b, 1.0 / 2.4) - 0.055);

    // RGB to HSV
    const max = Math.max(r, g, b);
    const min = Math.min(r, g, b);
    const d = max - min;

    let h = 0;
    const s = max === 0 ? 0 : (d / max) * 100;

    if (d !== 0) {
      if (max === r) h = ((g - b) / d + (g < b ? 6 : 0)) / 6;
      else if (max === g) h = ((b - r) / d + 2) / 6;
      else h = ((r - g) / d + 4) / 6;
    }

    return { hue: Math.round(h * 360), saturation: Math.round(s) };
  }

  function hsToXy(hue, saturation) {
    // HSV to RGB
    const h = hue / 360;
    const s = saturation / 100;
    const v = 1.0;

    const i = Math.floor(h * 6);
    const f = h * 6 - i;
    const p = v * (1 - s);
    const q = v * (1 - f * s);
    const t = v * (1 - (1 - f) * s);

    let r, g, b;
    switch (i % 6) {
      case 0: r = v; g = t; b = p; break;
      case 1: r = q; g = v; b = p; break;
      case 2: r = p; g = v; b = t; break;
      case 3: r = p; g = q; b = v; break;
      case 4: r = t; g = p; b = v; break;
      case 5: r = v; g = p; b = q; break;
    }

    // Reverse gamma
    r = r > 0.04045 ? Math.pow((r + 0.055) / 1.055, 2.4) : r / 12.92;
    g = g > 0.04045 ? Math.pow((g + 0.055) / 1.055, 2.4) : g / 12.92;
    b = b > 0.04045 ? Math.pow((b + 0.055) / 1.055, 2.4) : b / 12.92;

    // RGB to XY
    const X = r * 0.664511 + g * 0.154324 + b * 0.162028;
    const Y = r * 0.283881 + g * 0.668433 + b * 0.047685;
    const Z = r * 0.000088 + g * 0.072310 + b * 0.986039;

    const sum = X + Y + Z;
    if (sum === 0) return { x: 0, y: 0 };
    return { x: X / sum, y: Y / sum };
  }

  // Track last known state for set operations
  let lastHue = 0;
  let lastSaturation = 0;

  // ── Decode: Z2M JSON → HomeKit values ─────────────────────────
  function decode(message, info, output) {
    let payload;
    try {
      payload = JSON.parse(message);
    } catch (e) {
      return message; // Not JSON, return as-is
    }

    const prop = info.property;

    // On/Off state
    if (prop === 'on') {
      return payload.state === 'ON';
    }

    // Brightness: Z2M 0-254 → HomeKit 0-100
    if (prop === 'brightness') {
      return Math.round((payload.brightness / 254) * 100);
    }

    // Hue: extract from XY color
    if (prop === 'hue') {
      if (payload.color && payload.color.x !== undefined) {
        const hs = xyToHs(payload.color.x, payload.color.y);
        lastHue = hs.hue;
        lastSaturation = hs.saturation;
        return hs.hue;
      }
      return lastHue;
    }

    // Saturation: extract from XY color
    if (prop === 'saturation') {
      if (payload.color && payload.color.x !== undefined) {
        const hs = xyToHs(payload.color.x, payload.color.y);
        lastHue = hs.hue;
        lastSaturation = hs.saturation;
        return hs.saturation;
      }
      return lastSaturation;
    }

    // Color temperature: Z2M uses mireds, HomeKit uses mireds (140-500)
    if (prop === 'colorTemperature') {
      return payload.color_temp;
    }

    // Humidity (soil moisture sensors)
    if (prop === 'currentRelativeHumidity') {
      return payload.soil_moisture;
    }

    // Battery low status (< 10%)
    if (prop === 'statusLowBattery') {
      return payload.battery !== undefined && payload.battery < 10 ? 1 : 0;
    }

    // Contact sensor (garage door)
    if (prop === 'contactSensorState') {
      return payload.contact === false ? 1 : 0; // false = open = detected
    }

    return message;
  }

  // ── Encode: HomeKit values → Z2M JSON ─────────────────────────
  function encode(message, info, output) {
    const prop = info.property;

    // On/Off
    if (prop === 'on') {
      return JSON.stringify({ state: message ? 'ON' : 'OFF' });
    }

    // Brightness: HomeKit 0-100 → Z2M 0-254
    if (prop === 'brightness') {
      return JSON.stringify({ brightness: Math.round((message / 100) * 254) });
    }

    // Hue: convert HS to XY and send
    if (prop === 'hue') {
      lastHue = message;
      const xy = hsToXy(lastHue, lastSaturation);
      return JSON.stringify({ color: { x: xy.x, y: xy.y } });
    }

    // Saturation
    if (prop === 'saturation') {
      lastSaturation = message;
      const xy = hsToXy(lastHue, lastSaturation);
      return JSON.stringify({ color: { x: xy.x, y: xy.y } });
    }

    // Color temperature (mireds passthrough)
    if (prop === 'colorTemperature') {
      return JSON.stringify({ color_temp: message });
    }

    return message;
  }

  return { decode, encode };
}

module.exports = { init };
