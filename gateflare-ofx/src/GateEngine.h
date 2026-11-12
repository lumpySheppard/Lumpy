// GateEngine: host-independent film-gate edge flare.
//
// A lens throws an image circle bigger than the film gate. A bright source
// just outside the frame line still lands on the gate: light hits the edge of
// the aperture plate and scatters back onto the film, so a plume of light
// pours into the frame from the edge, right where the source is.
//
// To recreate it, the picture is cropped in slightly: the cropped-off margin
// stands for the part of the image circle beyond the gate. For each edge:
//   1. bright sources (linear light, above a threshold, optionally only small
//      lights) in a band around the edge are summed along it, from where a
//      light starts to flare (Flare Start) out to Reach beyond the edge;
//   2. that profile is thrown into the frame in the Look's shape (plume, beam
//      or blade), with a hot core and a long tail, streaks that radiate from
//      the light and shimmer over time, and a little colour fringing;
//   3. it's added as extra exposure on a negative (soft shoulder, never
//      clips), or screened / added, and the result is cropped to the gate.
//
// Top & bottom and left & right edges each get their own pair of Looks (A
// and B) with a blend between them.
#pragma once

#include <functional>
#include <vector>

namespace gate {

struct Image {
    int w = 0, h = 0;
    std::vector<float> px;  // RGBA, rows packed
    Image() = default;
    Image(int w_, int h_) : w(w_), h(h_), px(size_t(w_) * h_ * 4, 0.f) {}
    bool empty() const { return w <= 0 || h <= 0; }
    float* row(int y) { return px.data() + size_t(y) * w * 4; }
    const float* row(int y) const { return px.data() + size_t(y) * w * 4; }
};

enum Shape {
    kPlume = 0,  // a fan that widens into the frame
    kBeam = 1,   // narrow, parallel-sided column straight in from the edge
    kBlade = 2,  // hard-edged wedge, can be angled
    kPinch = 3,  // a tight, hot neck straight in from the edge that pinches, then fans out
                 // (the light focused by the gate edge, Gummo)
};

// How the flare is combined with the picture.
enum Composite {
    kFilm = 0,    // extra exposure through a soft shoulder (never clips)
    kScreen = 1,  // screen (display encodings); hits white on strong flares
    kAdd = 2,     // plain add in linear light (for log/scene-linear with an output transform)
};

// Everything about how a flare looks. Distances are % of frame height.
struct Look {
    int shape = kPlume;
    float intensity = 1.f;
    float flareStart = 0.f;    // where a light starts to flare: + still inside the frame, - past the edge
    float reach = 6.f;         // how far beyond the edge a light keeps flaring
    float depth = 14.f;        // how far the flare reaches into the frame
    float spread = 2.5f;       // width along the edge, where it starts
    float fan = 0.6f;          // how much wider it gets per unit of depth (plume, blade)
    float edgeGlow = 0.35f;    // thin glow right on the gate edge
    float bladeAngle = 15.f;   // degrees a blade leans away from straight in
    float bladeSoftness = 0.3f;  // blade side softness
    float streaks = 0.5f;      // striations from the gate edge, radiating from the light
    float streakDetail = 0.5f; // 0 = broad, 1 = fine
    float shimmer = 0.3f;      // how much the streaks and flare change frame to frame
    float shimmerSpeed = 1.f;
    float core = 0.4f;         // hot, tight core at the edge vs one even falloff
    float fringe = 0.3f;       // red scatters wider than blue
    float tint[3] = {1.f, 1.f, 1.f};
    float sourceColour = 1.f;  // 1 = the lights' own colour, 0 = white (then tinted)
    float farTint[3] = {1.f, 1.f, 1.f};  // colour it shifts toward as it goes into the frame
    float exitTint[3] = {1.f, 1.f, 1.f}; // colour it shifts toward as the light goes further out of frame
    float pinch = 15.f;        // pinch: length of the tight neck before it fans out
    float lean = 0.f;          // pinch: degrees the fan leans (+ = up on the sides, left on top/bottom)
    // Organic veiling-flare character.
    float falloff = 0.f;       // 0 = fades gradually from the edge, 1 = holds its strength, then drops away
    float curve = 0.f;         // bends the flare as it goes in (+ the same way as Lean)
    float veil = 0.f;          // a wide, low haze around the flare (veiling glare)
    float iridescence = 0.f;   // rainbow banding (thin-film / prism colours)
    float rainbowBands = 3.f;  // how many bands
    float rainbowAlong = 0.f;  // 0 = bands across the depth, 1 = along the flare (parallel to it)
    float prismEdges = 0.f;    // 1 = the colour only on the rays' edges (thin spectral lines)
};

// Built-in looks, from reference footage.
enum LookPreset {
    kLookCustom = 0,   // the Custom Look controls
    kSoftHaze,         // a soft, streaky plume (default character)
    kThinStreak,       // long, thin beam (a light leaving the side of frame)
    kCoolColumn,       // a cool column straight down from the top
    kBlueBlade,        // a hard-edged blue wedge
    kSideBlast,        // a tight blue neck that pinches and fans out, just after the light has gone
    kLookCount
};
const char* lookName(int preset);
// The preset's Look (or `custom` for kLookCustom).
Look presetLook(int preset, const Look& custom);

// An edge pair's flare: Look A, Look B and how much of B (0 = all A).
struct EdgePair {
    Look a, b;
    float blend = 0.f;
};

struct Settings {
    int encoding = 0;          // see shared/Encodings.h
    int composite = kFilm;
    EdgePair pair[2];          // 0 = top & bottom, 1 = left & right
    float time = 0.f;          // frame number (drives the shimmer)
    unsigned seed = 1;
    float crop = 6.f;          // % of the picture beyond the gate (zoom-in amount)
    bool cropOutput = true;    // scale the gate up to fill the frame
    float threshold = 0.9f;    // encoded value a source must exceed to flare
    float knee = 0.05f;        // soft knee width around the threshold
    float isolate = 1.f;       // 0 = anything bright flares; 1 = only small lights much
                               // brighter than their surroundings (bright walls drop out)
    float lightSize = 3.f;     // how big a "light" can be, % of frame height
    bool edges[4] = {true, true, true, true};  // top, bottom, left, right (as seen)
    bool showGate = false;     // debug: show the gate and the detected sources
    int upSign = 1;            // +1 if rows go bottom-up (OFX), -1 if top-down
};

void parallelFor(int n, const std::function<void(int)>& fn);

// Render `in` with gate flare into `out` (same size as `in`).
void render(const Image& in, const Settings& s, Image& out);

}  // namespace gate
