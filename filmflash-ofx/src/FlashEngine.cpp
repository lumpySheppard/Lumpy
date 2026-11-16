#include "FlashEngine.h"

#include "Encodings.h"

#include <mutex>
#include <exception>
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
    // An exception escaping a std::thread ends the whole process (the host
    // with it), so the workers hand the first one back to the caller.
    std::atomic<int> next{0};
    std::exception_ptr error;
    std::mutex errorLock;
    auto worker = [&] {
        try {
            for (int i; (i = next.fetch_add(1)) < n;) fn(i);
        } catch (...) {
            next.store(n);  // stop the others
            std::lock_guard<std::mutex> g(errorLock);
            if (!error) error = std::current_exception();
        }
    };
    std::vector<std::thread> pool;
    try {
        pool.reserve(size_t(nt - 1));
        for (int t = 1; t < nt; ++t) pool.emplace_back(worker);
    } catch (...) {
        // Couldn't start (all) the threads: the ones running, and this one, do the work.
    }
    worker();
    for (auto& t : pool) t.join();
    if (error) std::rethrow_exception(error);
}

namespace {

// Encodings live in ../shared/Encodings.h (shared with the other plugins).
using encodings::decode;
using encodings::encode;

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

    const bool screenAdd = lk.exposureMode == kGamma && !scene;
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
                const float ec = lk.response * e * (1.f + bias[c]);
                float o;
                if (lk.exposureMode == kFilmNegative) {
                    const float l = decode(s[x * 4 + c], lk.encoding);
                    if (scene) {
                        o = std::max(l, 0.f) / W;
                        O0[(size_t(y) * w + x) * 3 + c] = o;
                    } else {
                        float t = std::min(std::max(l, 0.f), 0.99995f);
                        o = t / (1.f - t);
                    }
                    o *= std::exp2(ec);
                } else if (lk.exposureMode == kGain) {
                    // Straight stops in linear light (unit = scene white W, or display 1).
                    o = decode(s[x * 4 + c], lk.encoding) / (scene ? W : 1.f) * std::exp2(ec);
                } else {
                    // Gamma on the encoded value: 1 stop ~ a gamma of 2^-0.35.
                    float v = std::max(s[x * 4 + c], 0.f);
                    o = decode(std::pow(v, std::exp2(-0.35f * ec)), lk.encoding) / (scene ? W : 1.f);
                }
                const float add = fogAmt * lk.fogColour[c] + leakAmt * leakW * lk.leakColour[c];
                // Gamma on a display encoding keeps whites put: screen the light in.
                o = screenAdd ? 1.f - (1.f - std::min(o, 1.f)) * (1.f - std::min(add, 1.f)) : o + add;
                O[(size_t(y) * w + x) * 3 + c] = o;
            }
        }
    });

    // Pass 2: halation. Bright areas (past the middle of the curve) bloom
    // through the base and come back mostly in the red layer.
    std::vector<float> halo;
    int hs = 4, hw = 0, hh = 0;
    // Film: past the middle of the curve (odds 1). Linear modes: half of white.
    const float halThreshold = lk.exposureMode == kFilmNegative ? 1.f : 0.5f;
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
                        acc += std::min(std::max(lum - halThreshold, 0.f), 40.f);
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
                const float hadd = halAmt * hv * halCol[c];
                float o = screenAdd ? 1.f - (1.f - O[(size_t(y) * w + x) * 3 + c]) * (1.f - std::min(hadd, 1.f))
                                    : O[(size_t(y) * w + x) * 3 + c] + hadd;
                if (lk.exposureMode != kFilmNegative) {
                    // No shoulder: linear light out, as far as the stops go.
                    dst[x * 4 + c] = encode(o * (scene ? W : 1.f), lk.encoding);
                    continue;
                }
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

float toLinear(float v, int encoding) { return decode(v, encoding); }
float fromLinear(float l, int encoding) { return encode(l, encoding); }

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

// ------------------------------------------------------------ motion blur

namespace {

// A smooth motion field from A to B on a coarse grid, in full-resolution
// pixels: the thing at p in A is at p + v(p) in B.
struct Flow {
    int gw = 0, gh = 0;  // grid nodes
    float step = 1.f;    // full-resolution pixels between nodes
    std::vector<float> vx, vy;
    float maxMove = 0.f;  // about the largest movement, in pixels
    void at(float x, float y, float& ox, float& oy) const {
        const float fx = std::min(std::max(x / step, 0.f), float(gw - 1));
        const float fy = std::min(std::max(y / step, 0.f), float(gh - 1));
        const int x0 = std::min(int(fx), gw - 2 < 0 ? 0 : gw - 2), y0 = std::min(int(fy), gh - 2 < 0 ? 0 : gh - 2);
        const int x1 = std::min(x0 + 1, gw - 1), y1 = std::min(y0 + 1, gh - 1);
        const float tx = fx - x0, ty = fy - y0;
        auto mix = [&](const std::vector<float>& a) {
            const float top = a[size_t(y0) * gw + x0] + (a[size_t(y0) * gw + x1] - a[size_t(y0) * gw + x0]) * tx;
            const float bot = a[size_t(y1) * gw + x0] + (a[size_t(y1) * gw + x1] - a[size_t(y1) * gw + x0]) * tx;
            return top + (bot - top) * ty;
        };
        ox = mix(vx);
        oy = mix(vy);
    }
};

struct Gray {
    int w = 0, h = 0;
    std::vector<float> v;
    float at(int x, int y) const {
        x = std::min(std::max(x, 0), w - 1);
        y = std::min(std::max(y, 0), h - 1);
        return v[size_t(y) * w + x];
    }
};

// Luma (as encoded, which is roughly how it looks), shrunk by `f`.
Gray shrink(const Image& im, int f) {
    Gray g;
    g.w = std::max(1, im.w / f);
    g.h = std::max(1, im.h / f);
    g.v.resize(size_t(g.w) * g.h);
    parallelFor(g.h, [&](int y) {
        for (int x = 0; x < g.w; ++x) {
            float acc = 0.f;
            int n = 0;
            for (int j = 0; j < f; ++j) {
                const int yy = y * f + j;
                if (yy >= im.h) break;
                const float* row = im.row(yy);
                for (int i = 0; i < f; ++i) {
                    const int xx = x * f + i;
                    if (xx >= im.w) break;
                    const float* p = row + size_t(xx) * 4;
                    acc += std::min(std::max(0.2126f * p[0] + 0.7152f * p[1] + 0.0722f * p[2], 0.f), 2.f);
                    ++n;
                }
            }
            g.v[size_t(y) * g.w + x] = n ? acc / n : 0.f;
        }
    });
    return g;
}

Gray halve(const Gray& a) {
    Gray g;
    g.w = std::max(1, a.w / 2);
    g.h = std::max(1, a.h / 2);
    g.v.resize(size_t(g.w) * g.h);
    for (int y = 0; y < g.h; ++y)
        for (int x = 0; x < g.w; ++x)
            g.v[size_t(y) * g.w + x] =
                0.25f * (a.at(2 * x, 2 * y) + a.at(2 * x + 1, 2 * y) + a.at(2 * x, 2 * y + 1) + a.at(2 * x + 1, 2 * y + 1));
    return g;
}

// Block matching, coarse to fine: a vector every `kNode` pixels of the small
// picture, each found by comparing a block around the node. A pull toward the
// neighbourhood's motion keeps featureless areas from wandering, and the
// field is median-filtered and smoothed at each level.
const int kNode = 4, kHalfBlock = 5;

Flow estimateFlow(const Image& A, const Image& B) {
    const int shrinkBy = std::max(1, int(std::lround(std::max(A.w, A.h) / 480.0)));
    std::vector<Gray> pa, pb;
    pa.push_back(shrink(A, shrinkBy));
    pb.push_back(shrink(B, shrinkBy));
    while (pa.size() < 4 && std::min(pa.back().w, pa.back().h) >= 48) {
        pa.push_back(halve(pa.back()));
        pb.push_back(halve(pb.back()));
    }
    std::vector<float> vx, vy;  // vectors at the level being worked on, in its pixels
    int gw = 0, gh = 0;
    for (int lv = int(pa.size()) - 1; lv >= 0; --lv) {
        const Gray& a = pa[size_t(lv)];
        const Gray& b = pb[size_t(lv)];
        const int nw = (a.w + kNode - 1) / kNode + 1, nh = (a.h + kNode - 1) / kNode + 1;
        // Start from the coarser level's field, twice as big.
        std::vector<float> px(size_t(nw) * nh, 0.f), py(size_t(nw) * nh, 0.f);
        if (gw > 0) {
            for (int y = 0; y < nh; ++y)
                for (int x = 0; x < nw; ++x) {
                    const float fx = std::min(x * 0.5f, float(gw - 1)), fy = std::min(y * 0.5f, float(gh - 1));
                    const int x0 = std::min(int(fx), std::max(gw - 2, 0)), y0 = std::min(int(fy), std::max(gh - 2, 0));
                    const int x1 = std::min(x0 + 1, gw - 1), y1 = std::min(y0 + 1, gh - 1);
                    const float tx = fx - x0, ty = fy - y0;
                    auto mix = [&](const std::vector<float>& v) {
                        const float top = v[size_t(y0) * gw + x0] + (v[size_t(y0) * gw + x1] - v[size_t(y0) * gw + x0]) * tx;
                        const float bot = v[size_t(y1) * gw + x0] + (v[size_t(y1) * gw + x1] - v[size_t(y1) * gw + x0]) * tx;
                        return 2.f * (top + (bot - top) * ty);
                    };
                    px[size_t(y) * nw + x] = mix(vx);
                    py[size_t(y) * nw + x] = mix(vy);
                }
        }
        const int range = gw > 0 ? 2 : 6;  // the coarsest level looks furthest
        std::vector<float> nx(px.size()), ny(py.size());
        parallelFor(nh, [&](int gy) {
            for (int gx = 0; gx < nw; ++gx) {
                const int cx = std::min(gx * kNode, a.w - 1), cy = std::min(gy * kNode, a.h - 1);
                const float p0x = px[size_t(gy) * nw + gx], p0y = py[size_t(gy) * nw + gx];
                const int bx = int(std::lround(p0x)), by = int(std::lround(p0y));
                float best = 1e30f;
                int bestX = bx, bestY = by;
                for (int dy = -range; dy <= range; ++dy)
                    for (int dx = -range; dx <= range; ++dx) {
                        const int ux = bx + dx, uy = by + dy;
                        float sad = 0.f;
                        for (int j = -kHalfBlock; j <= kHalfBlock && sad < best; ++j)
                            for (int i = -kHalfBlock; i <= kHalfBlock; ++i)
                                sad += std::fabs(a.at(cx + i, cy + j) - b.at(cx + i + ux, cy + j + uy));
                        // Prefer the expected motion when the picture can't tell.
                        sad += 0.012f * float((2 * kHalfBlock + 1) * (2 * kHalfBlock + 1)) *
                               std::sqrt(float(dx * dx + dy * dy));
                        if (sad < best) {
                            best = sad;
                            bestX = ux;
                            bestY = uy;
                        }
                    }
                nx[size_t(gy) * nw + gx] = float(bestX);
                ny[size_t(gy) * nw + gx] = float(bestY);
            }
        });
        // Median of the 3x3 neighbourhood, then a light blur: a smooth field.
        auto tidy = [&](std::vector<float>& v) {
            std::vector<float> m(v.size());
            for (int y = 0; y < nh; ++y)
                for (int x = 0; x < nw; ++x) {
                    float n9[9];
                    int k = 0;
                    for (int j = -1; j <= 1; ++j)
                        for (int i = -1; i <= 1; ++i)
                            n9[k++] = v[size_t(std::min(std::max(y + j, 0), nh - 1)) * nw + std::min(std::max(x + i, 0), nw - 1)];
                    std::nth_element(n9, n9 + 4, n9 + 9);
                    m[size_t(y) * nw + x] = n9[4];
                }
            for (int pass = 0; pass < 2; ++pass) {
                for (int y = 0; y < nh; ++y)
                    for (int x = 0; x < nw; ++x) {
                        float acc = 0.f;
                        for (int j = -1; j <= 1; ++j)
                            for (int i = -1; i <= 1; ++i)
                                acc += m[size_t(std::min(std::max(y + j, 0), nh - 1)) * nw + std::min(std::max(x + i, 0), nw - 1)];
                        v[size_t(y) * nw + x] = acc / 9.f;
                    }
                m = v;
            }
        };
        tidy(nx);
        tidy(ny);
        vx.swap(nx);
        vy.swap(ny);
        gw = nw;
        gh = nh;
    }
    Flow f;
    f.gw = gw;
    f.gh = gh;
    f.step = float(kNode * shrinkBy);
    f.vx.resize(vx.size());
    f.vy.resize(vy.size());
    std::vector<float> mags(vx.size());
    for (size_t i = 0; i < vx.size(); ++i) {
        f.vx[i] = vx[i] * shrinkBy;
        f.vy[i] = vy[i] * shrinkBy;
        mags[i] = std::sqrt(f.vx[i] * f.vx[i] + f.vy[i] * f.vy[i]);
    }
    if (!mags.empty()) {
        const size_t k = mags.size() * 98 / 100;
        std::nth_element(mags.begin(), mags.begin() + std::ptrdiff_t(k), mags.end());
        f.maxMove = mags[k];
    }
    return f;
}

// Bilinear sample of an RGBA image, clamped at the edges.
inline void sampleRGBA(const Image& im, float x, float y, float* o) {
    x = std::min(std::max(x, 0.f), float(im.w - 1));
    y = std::min(std::max(y, 0.f), float(im.h - 1));
    const int x0 = std::min(int(x), std::max(im.w - 2, 0)), y0 = std::min(int(y), std::max(im.h - 2, 0));
    const int x1 = std::min(x0 + 1, im.w - 1), y1 = std::min(y0 + 1, im.h - 1);
    const float tx = x - x0, ty = y - y0;
    const float* a = im.row(y0) + size_t(x0) * 4;
    const float* b = im.row(y0) + size_t(x1) * 4;
    const float* c = im.row(y1) + size_t(x0) * 4;
    const float* d = im.row(y1) + size_t(x1) * 4;
    for (int k = 0; k < 4; ++k) {
        const float top = a[k] + (b[k] - a[k]) * tx, bot = c[k] + (d[k] - c[k]) * tx;
        o[k] = top + (bot - top) * ty;
    }
}

}  // namespace

void exposeMotion(std::vector<Image>& frames, const std::vector<int>& times, double t, float width,
                  int encoding, Image& out) {
    if (frames.empty()) return;
    const size_t n = frames.size();
    auto nearest = [&] {
        size_t best = 0;
        for (size_t i = 1; i < n; ++i)
            if (std::fabs(times[i] - t) < std::fabs(times[best] - t)) best = i;
        return best;
    };
    // The exposure, limited to the frames there are.
    const double a = std::min(std::max(t - width / 2.0, double(times.front())), double(times.back()));
    const double b = std::min(std::max(t + width / 2.0, double(times.front())), double(times.back()));
    if (n == 1 || b - a < 1e-3) {
        out = std::move(frames[nearest()]);
        return;
    }
    // The stretch of each gap between frames that the exposure covers.
    struct Span {
        size_t i;        // frames[i] -> frames[i + 1]
        float lo, hi;    // part of the gap, 0..1
        float share;     // of the whole exposure
        Flow flow;
        int samples = 1;
    };
    std::vector<Span> spans;
    for (size_t i = 0; i + 1 < n; ++i) {
        const double t0 = times[i], t1 = times[i + 1];
        const double lo = std::max(a, t0), hi = std::min(b, t1);
        if (hi - lo <= 1e-6 || frames[i].w != frames[i + 1].w || frames[i].h != frames[i + 1].h) continue;
        Span sp;
        sp.i = i;
        sp.lo = float((lo - t0) / (t1 - t0));
        sp.hi = float((hi - t0) / (t1 - t0));
        sp.share = float((hi - lo) / (b - a));
        spans.push_back(std::move(sp));
    }
    if (spans.empty()) {
        out = std::move(frames[nearest()]);
        return;
    }
    // Motion first (on the pictures as they are), then to linear light.
    for (Span& sp : spans) sp.flow = estimateFlow(frames[sp.i], frames[sp.i + 1]);
    for (Image& f : frames)
        parallelFor(f.h, [&](int y) {
            float* p = f.row(y);
            for (int x = 0; x < f.w * 4; x += 4)
                for (int c = 0; c < 3; ++c) p[x + c] = decode(p[x + c], encoding);
        });
    // Enough in-between positions that neighbouring ones are about a pixel and
    // a half apart, within reason.
    int total = 0;
    for (Span& sp : spans) {
        sp.samples = std::min(std::max(int(std::ceil(sp.flow.maxMove * (sp.hi - sp.lo) / 1.5f)), 1), 24);
        total += sp.samples;
    }
    const int kMostSamples = 64;
    if (total > kMostSamples)
        for (Span& sp : spans) sp.samples = std::max(1, sp.samples * kMostSamples / total);

    const int w = frames[spans[0].i].w, h = frames[spans[0].i].h;
    Image res(w, h);
    for (const Span& sp : spans) {
        const Image& A = frames[sp.i];
        const Image& B = frames[sp.i + 1];
        const float wgt = sp.share / float(sp.samples);
        parallelFor(h, [&](int y) {
            float* d = res.row(y);
            for (int x = 0; x < w; ++x) {
                float vx, vy;
                sp.flow.at(float(x), float(y), vx, vy);
                float acc[4] = {0, 0, 0, 0};
                if (std::fabs(vx) + std::fabs(vy) < 0.05f) {
                    // Nothing moving here: the plain blend of the two frames.
                    const float s = 0.5f * (sp.lo + sp.hi);
                    const float* pa = A.row(y) + size_t(x) * 4;
                    const float* pb = B.row(y) + size_t(x) * 4;
                    for (int k = 0; k < 4; ++k) acc[k] = (pa[k] + (pb[k] - pa[k]) * s) * float(sp.samples);
                } else {
                    for (int q = 0; q < sp.samples; ++q) {
                        // The picture a fraction s of the way from A to B: what
                        // was s*v back in A, and what will be (1-s)*v on in B.
                        const float s = sp.lo + (sp.hi - sp.lo) * (float(q) + 0.5f) / float(sp.samples);
                        float ca[4], cb[4];
                        sampleRGBA(A, float(x) - s * vx, float(y) - s * vy, ca);
                        sampleRGBA(B, float(x) + (1.f - s) * vx, float(y) + (1.f - s) * vy, cb);
                        for (int k = 0; k < 4; ++k) acc[k] += ca[k] + (cb[k] - ca[k]) * s;
                    }
                }
                for (int k = 0; k < 4; ++k) d[size_t(x) * 4 + k] += acc[k] * wgt;
            }
        });
    }
    parallelFor(h, [&](int y) {
        float* p = res.row(y);
        for (int x = 0; x < w * 4; x += 4)
            for (int c = 0; c < 3; ++c) p[x + c] = encode(p[x + c], encoding);
    });
    out = std::move(res);
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
