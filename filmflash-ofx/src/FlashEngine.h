// FlashEngine: host-independent film run-out / run-up flash.
//
// When a film camera slows to a stop (or starts from one) the motor runs
// below speed, so every frame is exposed for longer:
//
//     exposure time = shutter angle / (360 * fps)
//
// and the extra exposure in stops is log2(normal fps / current fps), which
// creeps up and then shoots off as the camera stops. The frames bloom toward
// white the way a negative does: highlights slide into the shoulder instead
// of clipping, shadows go milky, saturation drains, the colour layers drift
// apart a little, and stray light fogs the base and halates around
// highlights.
//
// The tone model works in "odds" space, O = T / (1 - T) for display light T
// in [0, 1). Exposure multiplies O, and fog / halation add to it. Because
// T = O / (1 + O) can never reach 1, nothing clips; each extra stop halves
// the distance left to white. At 0 stops the image is returned unchanged.
#pragma once

#include <cstdint>
#include <functional>
#include <vector>

namespace flash {

struct Image {
    int w = 0, h = 0;
    std::vector<float> px;  // RGBA, rows packed
    Image() = default;
    Image(int w_, int h_) : w(w_), h(h_), px(size_t(w_) * h_ * 4, 0.f) {}
    bool empty() const { return w <= 0 || h <= 0; }
    float* row(int y) { return px.data() + size_t(y) * w * 4; }
    const float* row(int y) const { return px.data() + size_t(y) * w * 4; }
};

enum Encoding {
    kRec709 = 0,      // display, gamma 2.4 (Resolve's default timeline)
    kSRGB = 1,        // display, sRGB curve
    kGamma22 = 2,     // display, gamma 2.2
    kLinearDisplay = 3,
    kLinearScene = 4, // scene-referred linear
    kDaVinciIntermediate = 5,
    kCineon = 6,      // Cineon / film-scan log
};

inline bool isSceneReferred(int enc) {
    return enc == kLinearScene || enc == kDaVinciIntermediate || enc == kCineon;
}

struct Look {
    int encoding = kRec709;
    float response = 1.f;      // film contrast: how fast the flash builds per stop
    float colourShift = 0.5f;  // layers' differing sensitivity (warm drift)
    float highlightColour[3] = {1.f, 0.95f, 0.84f};  // what the flash burns toward
    float fog = 0.35f;         // base fog: milky, coloured lift of the blacks
    float fogColour[3] = {1.f, 0.62f, 0.32f};
    float halation = 0.4f;     // red glow around bright areas
    float halationRadius = 18.f;  // pixels at the image's current resolution
    float unevenness = 0.f;    // exposure varies across the frame (stops at the edge)
    float angle = 0.f;         // direction of the unevenness / leak, degrees (0 = from the left)
    float leak = 0.f;          // light leak on that edge
    float leakColour[3] = {1.f, 0.35f, 0.1f};
    float sceneCeiling = 6.f;  // scene-referred: stops above mid grey where it tops out
    int upSign = 1;            // +1 if image rows go bottom-up (OFX), -1 if top-down
};

// Runs fn(i) for i in [0, n) across hardware threads.
void parallelFor(int n, const std::function<void(int)>& fn);

// Expose `in` by `stops` extra stops through the film model into `out`.
// `out` may be `in`.
void render(const Image& in, float stops, const Look& look, Image& out);

// Transition envelope: extra stops at transition position p in [0, 1].
struct Envelope {
    float peak = 6.f;       // stops at the cut
    float cut = 0.5f;       // where the outgoing shot hands over to the incoming one
    float hold = 0.f;       // fraction of the transition held at the peak, around the cut
    float runOut = 1.f;     // shape of the build (higher = later, sharper rise)
    float runUp = 3.f;      // shape of the decay (higher = camera gets up to speed faster)
    float flicker = 0.f;    // random frame-to-frame variation, stops
    uint32_t seed = 1;
};
float envelopeStops(const Envelope& e, float p, int frame);

}  // namespace flash
