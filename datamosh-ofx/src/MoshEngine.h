// MoshEngine: host-independent datamosh simulation.
//
// Real datamoshing works inside a codec: a P-frame stores, for each
// macroblock, a motion vector pointing at where that block came from in the
// previous frame, plus a residual (the small correction the vector could not
// explain). Delete the I-frame at a cut and the decoder keeps applying the new
// shot's vectors and residuals to the old shot's pixels.
//
// This engine rebuilds that process on decoded frames:
//   1. estimateMotion() does codec-style block matching between two real
//      frames: a fixed grid of macroblocks, or a quadtree of mixed sizes
//      (4-64 px) like a modern encoder's partitions.
//   2. step() moves the moshed picture along that motion the way a decoder
//      builds a P-frame, adds the residual, and applies the optional damage:
//      stuck / scattered / streaked / flattened blocks and colour errors.
//   3. schedule() decides, frame by frame, which source frames drive the
//      motion, so motion can repeat, freeze, jump back/forward or reverse.
//
// Everything random is a pure function of (seed, frame, position), so any
// frame renders identically no matter what order a host asks for frames in.
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

struct Block {
    int x, y, size;
};

// Motion stored per cell (a cell is the smallest block size in the field).
// The block covering a cell came from offset (dx, dy) in the previous frame.
struct MotionField {
    int cell = 16;         // cell size in pixels
    int base = 16;         // nominal block size (for smoothing)
    int cw = 0, ch = 0;    // cells across / down
    int w = 0, h = 0;      // image size the field was built for
    std::vector<float> dx, dy;
    std::vector<Block> blocks;  // the partition (leaf blocks)
    bool empty() const { return cw == 0 || ch == 0; }
};

struct MotionSettings {
    int block = 16;        // base macroblock size in pixels
    int range = 24;        // max search distance in pixels
    float variety = 0.f;   // 0 = fixed grid; 1 = wildly mixed 4-64 px blocks
    uint32_t seed = 0;
};

struct GlitchSettings {
    float stuck = 0.f;     // chance a block stops updating
    float scatter = 0.f;   // chance a block grabs pixels from a random place
    float streak = 0.f;    // chance a block smears its top row down (drips)
    float flatten = 0.f;   // chance a block collapses to one flat colour
    float colourErr = 0.f; // chance a block gets a colour error (DC damage)
    float colourErrStrength = 0.5f;
    int hold = 6;          // frames a block glitch lasts
    int upSign = -1;       // +1 if visual "up" is +y (OFX images), -1 if -y
};

struct ColourSettings {
    float chromaDrift = 0.f;  // colour moves further than brightness
    float bleed = 0.f;        // boosts the residual's colour (wrong colours pile up)
    float satBuild = 0.f;     // saturation grows every frame
    float hueDrift = 0.f;     // degrees the hue rotates every frame
};

struct StepSettings {
    float motionScale = 1.f;  // multiply every vector (over-drive the smear)
    float residual = 0.3f;    // how much of the new shot's residual bleeds in
    float heal = 0.f;         // per-frame blend back toward the real frame
    bool smooth = false;      // interpolate vectors per pixel instead of per block
    int range = 24;           // used for scatter distance
    uint32_t seed = 0;
    GlitchSettings glitch;
    ColourSettings colour;
};

struct TimeSettings {
    float repeat = 0.f;    // chance per frame: re-apply the same motion for a while
    float freeze = 0.f;    // chance per frame: the picture holds still for a while
    float jumpBack = 0.f;  // chance per frame: motion jumps back in time
    float jumpFwd = 0.f;   // chance per frame: motion lurches forward
    float reverse = 0.f;   // chance per frame: motion runs backwards for a while
    int maxLen = 6;        // longest repeat/freeze/reverse, frames
    int maxJump = 12;      // longest jump, frames
    int every = 0;         // >0: events may only start every N frames (rhythm)
    uint32_t seed = 0;
    bool any() const { return repeat + freeze + jumpBack + jumpFwd + reverse > 0.f; }
};

// What one mosh frame does: move the picture using the motion of source
// frame `a` relative to source frame `b`, or (move == false) hold still.
struct StepSpec {
    bool move = true;
    int a = 0, b = 0;
};

// Steps for frames start+1 .. t (index k - start - 1). Without time glitches
// step k is {a = k, b = k - 1}.
std::vector<StepSpec> schedule(int start, int t, const TimeSettings& ts);

struct Scratch {
    Image predState, predDrift, predReal;
};

// Runs fn(i) for i in [0, n) across hardware threads.
void parallelFor(int n, const std::function<void(int)>& fn);

// Block-match `cur` against `prev`.
MotionField estimateMotion(const Image& cur, const Image& prev, const MotionSettings& s);

// out(p) = ref(p + scale * v(p)). `out` is resized to match `ref`.
void predict(const Image& ref, const MotionField& mf, float scale, bool smooth, Image& out);

// One decoded mosh frame.
//   state:    moshed picture, updated in place
//   resCur/resPrev: the real frames the motion came from (residual); may be null
//   healTo:   real frame at this output time (for Heal); may be null
//   k:        output frame number (drives the per-frame randomness)
void step(Image& state, const Image* resCur, const Image* resPrev, const Image* healTo,
          const MotionField& mf, const StepSettings& s, int k, Scratch& scratch);

// Cheap content fingerprint (sparse sample) so caches notice changed footage.
uint64_t fingerprint(const Image& img);

}  // namespace mosh
