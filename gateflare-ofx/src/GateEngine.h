// GateEngine: host-independent film-gate edge flare.
//
// A lens throws an image circle bigger than the film gate. A bright source
// just outside the frame line still lands on the gate: light hits the edge of
// the aperture plate and scatters back onto the film, so a soft plume pours
// into the frame from the edge, right where the source is. It shows up as a
// light approaches the edge, peaks as it crosses, and lingers just after it
// has left the shot.
//
// To recreate it, the picture is cropped in slightly: the cropped-off margin
// stands for the part of the image circle beyond the gate. For each edge:
//   1. bright sources (linear light, above a threshold) in a band straddling
//      the edge, mostly just outside it, are summed along the edge;
//   2. that profile is thrown into the frame as a plume that fades with
//      distance from the edge and widens (fans) as it goes, plus a thin glow
//      right on the edge;
//   3. it's added in linear light (screened, on display encodings, so whites
//      stay put), and the result is cropped to the gate.
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
    kPlume = 0,  // soft fan that widens into the frame
    kBeam = 1,   // narrow, parallel-sided column straight in from the edge
    kBlade = 2,  // hard-edged wedge, can be angled
};

// How the flare is combined with the picture.
enum Composite {
    kFilm = 0,    // extra exposure through a soft shoulder (never clips)
    kScreen = 1,  // screen (display encodings); hits white on strong flares
    kAdd = 2,     // plain add in linear light (for log/scene-linear with an output transform)
};

struct Settings {
    int encoding = 0;          // see shared/Encodings.h
    int composite = kFilm;
    int shape = kPlume;
    float bladeAngle = 15.f;   // degrees the blade leans away from straight-in
    float bladeSoftness = 0.3f;  // blade edge softness, % of frame height
    // Natural character: real gate flare isn't a smooth blob.
    float streaks = 0.5f;      // striations from the gate edge's nicks and roughness
    float streakDetail = 0.5f; // 0 = broad streaks, 1 = fine
    float shimmer = 0.3f;      // how much the streaks (and the flare) change frame to frame
    float shimmerSpeed = 1.f;
    float core = 0.4f;         // hot, tight core at the edge vs one soft falloff
    float fringe = 0.3f;       // red scatters wider than blue
    float time = 0.f;          // frame number (drives the shimmer)
    unsigned seed = 1;
    float crop = 6.f;          // % of the picture beyond the gate (zoom-in amount)
    bool cropOutput = true;    // scale the gate up to fill the frame
    float threshold = 0.9f;    // encoded value a source must exceed to flare
    float knee = 0.05f;        // soft knee width around the threshold
    float isolate = 1.f;      // 0 = anything bright flares; 1 = only small lights much
                               // brighter than their surroundings (bright walls drop out)
    float lightSize = 3.f;     // how big a "light" can be, % of frame height
    float intensity = 1.f;
    float reach = 6.f;         // how far beyond the edge sources still count, % of frame height
    float flareStart = 0.f;    // how far from the edge a light starts to flare, % of frame height:
                               // + while still inside the frame, - only once past the edge
    float depth = 14.f;        // how far the plume reaches into the frame, % of frame height
    float spread = 2.5f;       // plume width along the edge at the edge, % of frame height
    float fan = 0.6f;          // how much wider it gets per unit of depth
    float edgeGlow = 0.35f;    // thin glow right on the gate edge
    float tint[3] = {1.f, 1.f, 1.f};
    float sourceColour = 1.f;  // 1 = flare takes the sources' colour, 0 = white
    bool edges[4] = {true, true, true, true};  // top, bottom, left, right (as seen)
    bool showGate = false;     // debug: show the gate and the detected sources
    int upSign = 1;            // +1 if rows go bottom-up (OFX), -1 if top-down
};

void parallelFor(int n, const std::function<void(int)>& fn);

// Render `in` with gate flare into `out` (same size as `in`).
void render(const Image& in, const Settings& s, Image& out);

}  // namespace gate
