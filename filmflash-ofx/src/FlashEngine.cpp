#include "FlashEngine.h"

#include <algorithm>
#include <atomic>
#include <cmath>
#include <thread>

namespace flash {

void parallelFor(int n, const std::function<void(int)>& fn) {
    if (n <= 0) return;
    unsigned hw = std::thread::hardware_concurrency();
    int nt = std::max(1, std::min<int>(hw ? int(hw) : 4, n));
    if (nt == 1) {
        for (int i = 0; i < n; ++i) fn(i);
        return;
    }
    std::atomic<int> next{0};
    auto worker = [&] {
        for (int i; (i = next.fetch_add(1)) < n;) fn(i);
    };
    std::vector<std::thread> pool;
    for (int t = 1; t < nt; ++t) pool.emplace_back(worker);
    worker();
    for (auto& t : pool) t.join();
}

namespace {

// ------------------------------------------------------------ encodings

const float kCineonBlack = 0.0107977516f;  // 10^((95 - 685) / 300)

// DaVinci Intermediate constants (Blackmagic's published formula).
const float kDiA = 0.0075f, kDiB = 7.0f, kDiC = 0.07329248f, kDiM = 10.44426855f;
const float kDiLinCut = 0.00262409f, kDiLogCut = 0.02740668f;

inline float decode(float v, int enc) {
    switch (enc) {
        case kRec709: return std::pow(std::max(v, 0.f), 2.4f);
        case kSRGB:
            v = std::max(v, 0.f);
            return v <= 0.04045f ? v / 12.92f : std::pow((v + 0.055f) / 1.055f, 2.4f);
        case kGamma22: return std::pow(std::max(v, 0.f), 2.2f);
        case kDaVinciIntermediate:
            return v <= kDiLogCut ? v / kDiM : std::exp2(v / kDiC - kDiB) - kDiA;
        case kCineon:
            return (std::pow(10.f, (v * 1023.f - 685.f) / 300.f) - kCineonBlack) / (1.f - kCineonBlack);
        default: return v;  // linear
    }
}

inline float encode(float l, int enc) {
    switch (enc) {
        case kRec709: return std::pow(std::max(l, 0.f), 1.f / 2.4f);
        case kSRGB:
            l = std::max(l, 0.f);
            return l <= 0.0031308f ? l * 12.92f : 1.055f * std::pow(l, 1.f / 2.4f) - 0.055f;
        case kGamma22: return std::pow(std::max(l, 0.f), 1.f / 2.2f);
        case kDaVinciIntermediate:
            return l <= kDiLinCut ? l * kDiM : (std::log2(l + kDiA) + kDiB) * kDiC;
        case kCineon: {
            float x = std::max(l * (1.f - kCineonBlack) + kCineonBlack, 1e-10f);
            return (685.f + 300.f * std::log10(x)) / 1023.f;
        }
        default: return l;
    }
}

inline float smoothstep(float a, float b, float x) {
    float t = std::min(std::max((x - a) / (b - a), 0.f), 1.f);
    return t * t * (3.f - 2.f * t);
}

// Three box blurs ~ a gaussian. In place on a single-channel plane.
void boxBlur(std::vector<float>& p, int w, int h, int r) {
    if (r <= 0) return;
    std::vector<float> tmp(p.size());
    for (int pass = 0; pass < 3; ++pass) {
        parallelFor(h, [&](int y) {
            const float* s = &p[size_t(y) * w];
            float* d = &tmp[size_t(y) * w];
            float acc = 0;
            for (int i = -r; i <= r; ++i) acc += s[std::min(std::max(i, 0), w - 1)];
            for (int x = 0; x < w; ++x) {
                d[x] = acc / float(2 * r + 1);
                acc += s[std::min(x + r + 1, w - 1)] - s[std::max(x - r, 0)];
            }
        });
        parallelFor(w, [&](int x) {
            float acc = 0;
            for (int i = -r; i <= r; ++i) acc += tmp[size_t(std::min(std::max(i, 0), h - 1)) * w + x];
            for (int y = 0; y < h; ++y) {
                p[size_t(y) * w + x] = acc / float(2 * r + 1);
                acc += tmp[size_t(std::min(y + r + 1, h - 1)) * w + x] -
                       tmp[size_t(std::max(y - r, 0)) * w + x];
            }
        });
    }
}

inline uint64_t mix64(uint64_t x) {
    x += 0x9e3779b97f4a7c15ull;
    x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9ull;
    x = (x ^ (x >> 27)) * 0x94d049bb133111ebull;
    return x ^ (x >> 31);
}

}  // namespace

void render(const Image& in, float stops, const Look& lk, Image& out) {
    if (&out != &in && (out.w != in.w || out.h != in.h)) out = Image(in.w, in.h);
    if (in.empty()) return;
    if (stops == 0.f) {
        if (&out != &in) out.px = in.px;
        return;
    }
    const int w = in.w, h = in.h;
    const bool scene = isSceneReferred(lk.encoding);
    const float W = 0.18f * std::exp2(lk.sceneCeiling);
    // How "on" the flash's side effects are: 0 at no flash, -> 1 as it builds.
    const float kE = stops > 0.f ? 1.f - std::exp2(-stops / 2.f) : 0.f;
    const float pos = std::max(stops, 0.f);
    const float fogAmt = lk.fog * 0.035f * (std::exp2(0.6f * pos) - 1.f);
    const float leakAmt = lk.leak * 0.06f * (std::exp2(0.7f * pos) - 1.f);
    const float bias[3] = {0.12f * lk.colourShift, 0.f, -0.2f * lk.colourShift};
    const float ang = lk.angle * 3.14159265f / 180.f;
    const float dirx = -std::cos(ang), diry = std::sin(ang);  // toward the bright side

    // Pass 1: exposure + fog + leak in odds space.
    std::vector<float> O(size_t(w) * h * 3), O0(scene ? size_t(w) * h * 3 : 0);
    parallelFor(h, [&](int y) {
        const float* s = in.row(y);
        const float vy = lk.upSign > 0 ? ((y + 0.5f) / h - 0.5f) * 2.f : (0.5f - (y + 0.5f) / h) * 2.f;
        for (int x = 0; x < w; ++x) {
            const float vx = ((x + 0.5f) / w - 0.5f) * 2.f;
            const float d = std::min(std::max(vx * dirx + vy * diry, -1.f), 1.f);
            const float e = stops + lk.unevenness * d * kE;
            const float leakW = leakAmt > 0.f ? smoothstep(0.25f, 1.05f, d) : 0.f;
            for (int c = 0; c < 3; ++c) {
                const float l = decode(s[x * 4 + c], lk.encoding);
                float o;
                if (scene) {
                    o = std::max(l, 0.f) / W;
                    O0[(size_t(y) * w + x) * 3 + c] = o;
                } else {
                    float t = std::min(std::max(l, 0.f), 0.99995f);
                    o = t / (1.f - t);
                }
                o *= std::exp2(lk.response * e * (1.f + bias[c]));
                o += fogAmt * lk.fogColour[c] + leakAmt * leakW * lk.leakColour[c];
                O[(size_t(y) * w + x) * 3 + c] = o;
            }
        }
    });

    // Pass 2: halation. Bright areas (past the middle of the curve) bloom
    // through the base and come back mostly in the red layer.
    std::vector<float> halo;
    int hs = 4, hw = 0, hh = 0;
    if (lk.halation > 0.f && kE > 0.f && lk.halationRadius > 0.f) {
        hw = (w + hs - 1) / hs;
        hh = (h + hs - 1) / hs;
        halo.assign(size_t(hw) * hh, 0.f);
        parallelFor(hh, [&](int by) {
            for (int bx = 0; bx < hw; ++bx) {
                float acc = 0;
                int n = 0;
                for (int y = by * hs; y < std::min(h, by * hs + hs); ++y)
                    for (int x = bx * hs; x < std::min(w, bx * hs + hs); ++x, ++n) {
                        const float* o = &O[(size_t(y) * w + x) * 3];
                        float lum = 0.2126f * o[0] + 0.7152f * o[1] + 0.0722f * o[2];
                        acc += std::min(std::max(lum - 1.f, 0.f), 40.f);
                    }
                halo[size_t(by) * hw + bx] = n ? acc / n : 0.f;
            }
        });
        boxBlur(halo, hw, hh, std::max(1, int(std::lround(lk.halationRadius / hs / 1.7f))));
    }
    const float halCol[3] = {1.f, 0.3f, 0.07f};
    const float halAmt = lk.halation * 0.3f * kE;

    // Pass 3: back through the curve, toward the highlight colour, re-encode.
    out.w = w;
    out.h = h;
    out.px.resize(size_t(w) * h * 4);
    parallelFor(h, [&](int y) {
        const float* s = in.row(y);
        float* dst = out.row(y);
        float hy = (y + 0.5f) / hs - 0.5f;
        for (int x = 0; x < w; ++x) {
            float hv = 0.f;
            if (!halo.empty()) {
                float hx = (x + 0.5f) / hs - 0.5f;
                float cx = std::min(std::max(hx, 0.f), float(hw - 1));
                float cy = std::min(std::max(hy, 0.f), float(hh - 1));
                int x0 = int(cx), y0 = int(cy);
                int x1 = std::min(x0 + 1, hw - 1), y1 = std::min(y0 + 1, hh - 1);
                float tx = cx - x0, ty = cy - y0;
                float a = halo[size_t(y0) * hw + x0], b = halo[size_t(y0) * hw + x1];
                float c = halo[size_t(y1) * hw + x0], d = halo[size_t(y1) * hw + x1];
                hv = (a + (b - a) * tx) + ((c + (d - c) * tx) - (a + (b - a) * tx)) * ty;
            }
            const float alpha = s[x * 4 + 3];
            for (int c = 0; c < 3; ++c) {
                float o = O[(size_t(y) * w + x) * 3 + c] + halAmt * hv * halCol[c];
                o = std::max(o, 0.f);
                float t = o / (1.f + o);  // the shoulder: never reaches 1
                // Near the top the flash burns toward the stock's highlight colour.
                float tint = 1.f - kE * t * t * (1.f - lk.highlightColour[c]);
                float l;
                if (scene) {
                    float o0 = O0[(size_t(y) * w + x) * 3 + c];
                    l = W * (1.f + o0) * t * tint;  // equals the input when nothing changed
                } else {
                    l = t * tint;
                }
                dst[x * 4 + c] = encode(l, lk.encoding);
            }
            dst[x * 4 + 3] = alpha;
        }
    });
}

float envelopeStops(const Envelope& e, float p, int frame) {
    p = std::min(std::max(p, 0.f), 1.f);
    const float c = std::min(std::max(e.cut, 0.001f), 0.999f);
    const float hh = std::max(e.hold, 0.f) * 0.5f;
    float u;
    if (std::fabs(p - c) <= hh) {
        u = 1.f;
    } else if (p < c) {
        float span = c - hh;
        u = span > 0.f ? std::pow(std::min(std::max(p / span, 0.f), 1.f), std::max(e.runOut, 0.05f)) : 1.f;
    } else {
        float span = 1.f - c - hh;
        u = span > 0.f ? std::pow(std::min(std::max((1.f - p) / span, 0.f), 1.f), std::max(e.runUp, 0.05f)) : 1.f;
    }
    // Camera speed falls from full to 2^-peak of full; exposure follows 1/speed.
    const float r = std::exp2(-std::max(e.peak, 0.f));
    float stops = -std::log2(1.f - (1.f - r) * u);
    if (e.flicker > 0.f && u > 0.f) {
        uint64_t hsh = mix64(mix64(e.seed) ^ uint64_t(int64_t(frame) + (int64_t(1) << 40)));
        float rr = float(hsh >> 40) / 16777216.f * 2.f - 1.f;
        stops += e.flicker * rr * std::min(1.f, u * 2.f);
    }
    return std::max(stops, 0.f);
}

float speedFactor(const Speed& s, float stops) {
    if (s.speedUp <= 0.f || stops <= 0.f) return 1.f;
    return std::min(std::max(s.maxSpeed, 1.f), std::exp2(s.speedUp * stops));
}

float blurFrames(const Speed& s, float stops) {
    if (s.blur <= 0.f || stops <= 0.f || s.maxBlur <= 0.f) return 0.f;
    float w = (s.shutter / 360.f) * (std::exp2(s.blur * stops) - 1.f);
    return std::min(w, s.maxBlur);
}

std::vector<std::pair<int, float>> blurTaps(double t, float width) {
    std::vector<std::pair<int, float>> taps;
    if (width < 1e-3f) {
        taps.emplace_back(int(std::floor(t + 0.5)), 1.f);
        return taps;
    }
    const double a = t - width / 2.0, b = t + width / 2.0;
    double total = 0;
    for (int k = int(std::floor(a + 0.5)); k <= int(std::ceil(b - 0.5)); ++k) {
        double ov = std::min(k + 0.5, b) - std::max(k - 0.5, a);
        if (ov <= 0) continue;
        taps.emplace_back(k, float(ov));
        total += ov;
    }
    for (auto& tp : taps) tp.second = float(tp.second / total);
    return taps;
}

void blendFrames(const std::vector<const Image*>& frames, const std::vector<float>& weights,
                 int encoding, Image& out) {
    if (frames.empty()) return;
    const Image& f0 = *frames[0];
    if (frames.size() == 1) {
        if (&out != &f0) out = f0;
        return;
    }
    Image res(f0.w, f0.h);
    parallelFor(f0.h, [&](int y) {
        float* d = res.row(y);
        for (int x = 0; x < f0.w * 4; x += 4) {
            float acc[4] = {0, 0, 0, 0};
            for (size_t i = 0; i < frames.size(); ++i) {
                const float* s = frames[i]->row(y) + x;
                for (int c = 0; c < 3; ++c) acc[c] += weights[i] * decode(s[c], encoding);
                acc[3] += weights[i] * s[3];
            }
            for (int c = 0; c < 3; ++c) d[x + c] = encode(acc[c], encoding);
            d[x + 3] = acc[3];
        }
    });
    out = std::move(res);
}

}  // namespace flash
