// motion_blur_check: the motion blur must be a continuous smear, not a stack
// of separate frames.
//
// A bright square crosses a dim, textured background at 24 pixels a frame.
// Exposed over 4 frames, a row through its path should ramp up, hold and ramp
// down smoothly. Averaging whole frames instead gives a staircase: the edge of
// each copy of the square is a jump of a quarter of its brightness.
#include <cmath>
#include <cstdio>
#include <vector>

#include "FlashEngine.h"

using namespace flash;

namespace {
const int W = 320, H = 180, kSpeed = 24, kSize = 56;

Image frameAt(int k, bool moving) {
    Image im(W, H);
    const int x0 = 60 + (moving ? k * kSpeed : 0), y0 = 60;
    for (int y = 0; y < H; ++y)
        for (int x = 0; x < W; ++x) {
            float v = 0.12f + 0.03f * std::sin(x * 0.37f) * std::cos(y * 0.29f);
            if (x >= x0 && x < x0 + kSize && y >= y0 && y < y0 + kSize)
                v = 0.8f + 0.06f * std::sin((x - x0) * 0.9f) * std::sin((y - y0) * 0.7f);
            float* p = im.row(y) + size_t(x) * 4;
            p[0] = p[1] = p[2] = v;
            p[3] = 1.f;
        }
    return im;
}

// The biggest step between neighbouring pixels along the row through the square.
float biggestStep(const Image& im) {
    float worst = 0.f;
    const float* row = im.row(60 + kSize / 2);
    for (int x = 1; x < W; ++x) worst = std::max(worst, std::fabs(row[size_t(x) * 4] - row[size_t(x - 1) * 4]));
    return worst;
}
}  // namespace

int main() {
    int failures = 0;
    const float width = 4.f;
    const double centre = 2.0;

    // Whole frames averaged (the old way), for comparison.
    std::vector<Image> held;
    std::vector<const Image*> ptrs;
    std::vector<float> wts;
    for (auto& tp : blurTaps(centre, width)) {
        held.push_back(frameAt(tp.first, true));
        wts.push_back(tp.second);
    }
    for (auto& i : held) ptrs.push_back(&i);
    Image stacked;
    blendFrames(ptrs, wts, kRec709, stacked);

    std::vector<Image> frames;
    std::vector<int> times;
    for (int k = 0; k <= 4; ++k) {
        frames.push_back(frameAt(k, true));
        times.push_back(k);
    }
    Image smooth;
    exposeMotion(frames, times, centre, width, kRec709, smooth);

    const float a = biggestStep(stacked), b = biggestStep(smooth);
    std::printf("biggest step along the motion: whole frames averaged %.3f, continuous %.3f\n", a, b);
    if (!(a > 0.12f)) { std::printf("FAIL: the comparison isn't stepping (bad test)\n"); ++failures; }
    if (!(b < 0.06f)) { std::printf("FAIL: the motion blur shows separate frames\n"); ++failures; }

    // The light is conserved: the row's average is about the same either way.
    double sa = 0, sb = 0;
    for (int x = 0; x < W; ++x) {
        sa += toLinear(stacked.row(88)[size_t(x) * 4], kRec709);
        sb += toLinear(smooth.row(88)[size_t(x) * 4], kRec709);
    }
    std::printf("light along the row: %.2f vs %.2f\n", sa, sb);
    if (std::fabs(sa - sb) > 0.06 * sa) { std::printf("FAIL: the blur gained or lost light\n"); ++failures; }

    // Nothing moving: the picture is left alone.
    std::vector<Image> still;
    for (int k = 0; k <= 4; ++k) still.push_back(frameAt(0, false));
    const Image ref = frameAt(0, false);
    Image out;
    exposeMotion(still, times, centre, width, kRec709, out);
    float worst = 0.f;
    for (size_t i = 0; i < ref.px.size(); ++i) worst = std::max(worst, std::fabs(ref.px[i] - out.px[i]));
    std::printf("still picture: biggest change %.5f\n", worst);
    if (worst > 2e-3f) { std::printf("FAIL: a still picture changed\n"); ++failures; }

    // No exposure width, or one frame: that frame.
    std::vector<Image> one;
    one.push_back(frameAt(3, true));
    Image single;
    exposeMotion(one, {3}, 3.0, 4.f, kRec709, single);
    if (single.px != frameAt(3, true).px) { std::printf("FAIL: a single frame wasn't passed through\n"); ++failures; }

    std::printf(failures ? "FAILED\n" : "OK\n");
    return failures ? 1 : 0;
}
