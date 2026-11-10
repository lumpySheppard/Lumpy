// MoshEngine: host-independent datamosh simulation.
//
// Real datamoshing works inside a codec: a P-frame stores, for each
// macroblock, a motion vector pointing at where that block came from in the
// previous frame, plus a residual (the small correction the vector could not
// explain). Delete the I-frame at a cut and the decoder keeps applying the new
// shot's vectors and residuals to the old shot's pixels.
//
// This engine rebuilds that process on decoded frames:
//   1. estimateMotion() does codec-style block matching (SAD search over a
//      luma pyramid) between two real frames, giving one vector per block.
//   2. predict() moves an image along a motion field, the way a decoder
//      builds a P-frame prediction.
//   3. stepDrag()/stepBloom() apply one decoded frame's worth of moshing.
#pragma once

#include <cstdint>
#include <functional>
#include <vector>

namespace mosh {

// RGBA float image, rows packed, row 0 first.
struct Image {
    int w = 0, h = 0;
    std::vector<float> px;  // w * h * 4

    Image() = default;
    Image(int w_, int h_) : w(w_), h(h_), px(size_t(w_) * h_ * 4, 0.f) {}
    bool empty() const { return w <= 0 || h <= 0; }
    float* row(int y) { return px.data() + size_t(y) * w * 4; }
    const float* row(int y) const { return px.data() + size_t(y) * w * 4; }
};

// One vector per block: the block at (bx, by) in the current frame is best
// matched by the region offset by (dx, dy) in the previous frame.
struct MotionField {
    int block = 16;
    int bw = 0, bh = 0;    // blocks across / down
    int w = 0, h = 0;      // image size the field was built for
    std::vector<float> dx, dy;
    bool empty() const { return bw == 0 || bh == 0; }
};

struct MotionSettings {
    int block = 16;      // macroblock size in pixels
    int range = 24;      // max search distance in pixels
};

struct StepSettings {
    float motionScale = 1.f;  // multiply every vector (over-drive the smear)
    float residual = 0.25f;   // how much of the new shot's residual bleeds in
    float heal = 0.f;         // per-frame blend back toward the real frame
    bool smooth = false;      // interpolate vectors per pixel instead of per block
};

// Runs fn(i) for i in [0, n) across hardware threads.
void parallelFor(int n, const std::function<void(int)>& fn);

// Block-match `cur` against `prev`.
MotionField estimateMotion(const Image& cur, const Image& prev, const MotionSettings& s);

// out(p) = ref(p + scale * v(p)). `out` is resized to match `ref`.
void predict(const Image& ref, const MotionField& mf, float scale, bool smooth, Image& out);

// One "dropped I-frame" step.
//   state:   the moshed picture at frame k-1, updated in place to frame k
//   cur:     real frame k
//   prev:    real frame k-1 (used for the residual)
//   mf:      estimateMotion(cur, prev)
void stepDrag(Image& state, const Image& cur, const Image& prev, const MotionField& mf,
              const StepSettings& s, Image& scratchA, Image& scratchB);

// One "repeated P-frame" step: re-apply a frozen motion field to the state.
// `cur` is the real frame at this time (only used for healing).
void stepBloom(Image& state, const Image& cur, const MotionField& mf,
               const StepSettings& s, Image& scratch);

// Cheap content fingerprint (sparse sample) so caches notice changed footage.
uint64_t fingerprint(const Image& img);

}  // namespace mosh
