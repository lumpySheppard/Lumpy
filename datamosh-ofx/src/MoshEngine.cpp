#include "MoshEngine.h"

#include <algorithm>
#include <atomic>
#include <cmath>
#include <cstring>
#include <limits>
#include <thread>

namespace mosh {

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
    pool.reserve(nt - 1);
    for (int t = 1; t < nt; ++t) pool.emplace_back(worker);
    worker();
    for (auto& t : pool) t.join();
}

namespace {

struct Plane {
    int w = 0, h = 0;
    std::vector<float> v;
    float at(int x, int y) const { return v[size_t(y) * w + x]; }
};

Plane lumaOf(const Image& img) {
    Plane p;
    p.w = img.w;
    p.h = img.h;
    p.v.resize(size_t(p.w) * p.h);
    parallelFor(img.h, [&](int y) {
        const float* s = img.row(y);
        float* d = p.v.data() + size_t(y) * p.w;
        for (int x = 0; x < img.w; ++x, s += 4) {
            // Rec.709 luma; clamp so super-whites don't dominate the match.
            float l = 0.2126f * s[0] + 0.7152f * s[1] + 0.0722f * s[2];
            d[x] = std::min(std::max(l, 0.f), 2.f);
        }
    });
    return p;
}

Plane half(const Plane& s) {
    Plane d;
    d.w = std::max(1, s.w / 2);
    d.h = std::max(1, s.h / 2);
    d.v.resize(size_t(d.w) * d.h);
    for (int y = 0; y < d.h; ++y) {
        int y0 = std::min(2 * y, s.h - 1), y1 = std::min(2 * y + 1, s.h - 1);
        for (int x = 0; x < d.w; ++x) {
            int x0 = std::min(2 * x, s.w - 1), x1 = std::min(2 * x + 1, s.w - 1);
            d.v[size_t(y) * d.w + x] =
                0.25f * (s.at(x0, y0) + s.at(x1, y0) + s.at(x0, y1) + s.at(x1, y1));
        }
    }
    return d;
}

// SAD of the block [x0,x1)x[y0,y1) in `cur` against `prev` shifted by (vx,vy).
// `limit` lets a search abandon a candidate early once it is already worse.
inline float blockSAD(const Plane& cur, const Plane& prev, int x0, int y0, int x1, int y1,
                      int vx, int vy, float limit) {
    float sad = 0.f;
    bool inside = x0 + vx >= 0 && y0 + vy >= 0 && x1 + vx <= prev.w && y1 + vy <= prev.h;
    for (int y = y0; y < y1; ++y) {
        const float* c = cur.v.data() + size_t(y) * cur.w;
        if (inside) {
            const float* p = prev.v.data() + size_t(y + vy) * prev.w + vx;
            for (int x = x0; x < x1; ++x) sad += std::fabs(c[x] - p[x]);
        } else {
            int py = std::min(std::max(y + vy, 0), prev.h - 1);
            const float* p = prev.v.data() + size_t(py) * prev.w;
            for (int x = x0; x < x1; ++x) {
                int px = std::min(std::max(x + vx, 0), prev.w - 1);
                sad += std::fabs(c[x] - p[px]);
            }
        }
        if (sad > limit) return sad;
    }
    return sad;
}

}  // namespace

MotionField estimateMotion(const Image& cur, const Image& prev, const MotionSettings& s) {
    MotionField mf;
    if (cur.empty() || prev.w != cur.w || prev.h != cur.h) return mf;

    const int block = std::max(4, s.block);
    const int range = std::max(1, s.range);
    mf.block = block;
    mf.w = cur.w;
    mf.h = cur.h;
    mf.bw = (cur.w + block - 1) / block;
    mf.bh = (cur.h + block - 1) / block;
    const int nb = mf.bw * mf.bh;
    mf.dx.assign(nb, 0.f);
    mf.dy.assign(nb, 0.f);

    // Pyramid: search coarsely, then refine. Keep coarse blocks >= 4 px.
    int levels = 0;
    while (levels < 3 && (block >> (levels + 1)) >= 4 && (cur.w >> (levels + 1)) >= 32 &&
           (cur.h >> (levels + 1)) >= 32)
        ++levels;

    std::vector<Plane> pc{lumaOf(cur)}, pp{lumaOf(prev)};
    for (int l = 0; l < levels; ++l) {
        pc.push_back(half(pc.back()));
        pp.push_back(half(pp.back()));
    }

    std::vector<int> vx(nb, 0), vy(nb, 0);

    for (int l = levels; l >= 0; --l) {
        const Plane& C = pc[l];
        const Plane& P = pp[l];
        const int bs = block >> l;
        // Penalise long vectors slightly, like a codec's rate term: static and
        // flat areas keep a zero vector instead of jittering.
        const float lambda = 0.004f * bs * bs;
        const bool coarsest = (l == levels);
        const int r = coarsest ? (range + (1 << l) - 1) >> l : 1;

        parallelFor(mf.bh, [&](int by) {
            for (int bx = 0; bx < mf.bw; ++bx) {
                int i = by * mf.bw + bx;
                int x0 = bx * bs, y0 = by * bs;
                int x1 = std::min(x0 + bs, C.w), y1 = std::min(y0 + bs, C.h);
                if (x0 >= C.w || y0 >= C.h) {  // block vanished at this level
                    if (!coarsest) { vx[i] *= 2; vy[i] *= 2; }
                    continue;
                }
                int cx = coarsest ? 0 : vx[i] * 2;
                int cy = coarsest ? 0 : vy[i] * 2;

                auto cost = [&](int ux, int uy, float limit) {
                    float pen = lambda * float(std::abs(ux) + std::abs(uy));
                    return blockSAD(C, P, x0, y0, x1, y1, ux, uy, limit - pen) + pen;
                };

                int bestx = cx, besty = cy;
                float best = cost(cx, cy, std::numeric_limits<float>::max());
                // Static block: good enough, skip the search.
                float area = float((x1 - x0) * (y1 - y0));
                if (coarsest && best < 0.002f * area) {
                    vx[i] = 0; vy[i] = 0;
                    continue;
                }
                for (int dy = -r; dy <= r; ++dy) {
                    for (int dx = -r; dx <= r; ++dx) {
                        int ux = cx + dx, uy = cy + dy;
                        if (ux == cx && uy == cy) continue;
                        float c = cost(ux, uy, best);
                        if (c < best) { best = c; bestx = ux; besty = uy; }
                    }
                }
                vx[i] = bestx;
                vy[i] = besty;
            }
        });
    }

    for (int i = 0; i < nb; ++i) {
        mf.dx[i] = float(vx[i]);
        mf.dy[i] = float(vy[i]);
    }
    return mf;
}

namespace {

inline void sampleBilinear(const Image& img, float fx, float fy, float* out) {
    fx = std::min(std::max(fx, 0.f), float(img.w - 1));
    fy = std::min(std::max(fy, 0.f), float(img.h - 1));
    int x0 = int(fx), y0 = int(fy);
    int x1 = std::min(x0 + 1, img.w - 1), y1 = std::min(y0 + 1, img.h - 1);
    float tx = fx - x0, ty = fy - y0;
    const float* a = img.row(y0) + x0 * 4;
    const float* b = img.row(y0) + x1 * 4;
    const float* c = img.row(y1) + x0 * 4;
    const float* d = img.row(y1) + x1 * 4;
    for (int k = 0; k < 4; ++k) {
        float top = a[k] + (b[k] - a[k]) * tx;
        float bot = c[k] + (d[k] - c[k]) * tx;
        out[k] = top + (bot - top) * ty;
    }
}

// Vector for pixel (x, y), either the block's own or interpolated between
// neighbouring block centres.
inline void vectorAt(const MotionField& mf, int x, int y, bool smooth, float& ox, float& oy) {
    if (!smooth) {
        int bx = std::min(x / mf.block, mf.bw - 1);
        int by = std::min(y / mf.block, mf.bh - 1);
        int i = by * mf.bw + bx;
        ox = mf.dx[i];
        oy = mf.dy[i];
        return;
    }
    float gx = (x + 0.5f) / mf.block - 0.5f;
    float gy = (y + 0.5f) / mf.block - 0.5f;
    gx = std::min(std::max(gx, 0.f), float(mf.bw - 1));
    gy = std::min(std::max(gy, 0.f), float(mf.bh - 1));
    int x0 = int(gx), y0 = int(gy);
    int x1 = std::min(x0 + 1, mf.bw - 1), y1 = std::min(y0 + 1, mf.bh - 1);
    float tx = gx - x0, ty = gy - y0;
    auto lerp2 = [&](const std::vector<float>& f) {
        float a = f[y0 * mf.bw + x0], b = f[y0 * mf.bw + x1];
        float c = f[y1 * mf.bw + x0], d = f[y1 * mf.bw + x1];
        float top = a + (b - a) * tx, bot = c + (d - c) * tx;
        return top + (bot - top) * ty;
    };
    ox = lerp2(mf.dx);
    oy = lerp2(mf.dy);
}

}  // namespace

void predict(const Image& ref, const MotionField& mf, float scale, bool smooth, Image& out) {
    if (out.w != ref.w || out.h != ref.h) out = Image(ref.w, ref.h);
    if (mf.empty() || mf.w != ref.w || mf.h != ref.h) {
        out.px = ref.px;
        return;
    }
    parallelFor(ref.h, [&](int y) {
        float* d = out.row(y);
        for (int x = 0; x < ref.w; ++x, d += 4) {
            float vx, vy;
            vectorAt(mf, x, y, smooth, vx, vy);
            sampleBilinear(ref, x + vx * scale, y + vy * scale, d);
        }
    });
}

namespace {

float maxValue(const Image& img) {
    float m = 1.f;
    for (size_t i = 0; i < img.px.size(); i += 4)
        m = std::max(m, std::max(img.px[i], std::max(img.px[i + 1], img.px[i + 2])));
    return m;
}

}  // namespace

void stepDrag(Image& state, const Image& cur, const Image& prev, const MotionField& mf,
              const StepSettings& s, Image& predState, Image& predReal) {
    predict(state, mf, s.motionScale, s.smooth, predState);
    const bool useResidual = s.residual > 0.f && prev.w == cur.w && prev.h == cur.h;
    if (useResidual) predict(prev, mf, s.motionScale, s.smooth, predReal);
    const float hi = maxValue(cur);

    if (state.w != cur.w || state.h != cur.h) state = Image(cur.w, cur.h);
    parallelFor(cur.h, [&](int y) {
        const float* ps = predState.row(y);
        const float* pr = useResidual ? predReal.row(y) : nullptr;
        const float* c = cur.row(y);
        float* o = state.row(y);
        for (int x = 0; x < cur.w; ++x, ps += 4, c += 4, o += 4) {
            for (int k = 0; k < 3; ++k) {
                float v = ps[k];
                if (pr) v += s.residual * (c[k] - pr[k]);
                v += (c[k] - v) * s.heal;
                o[k] = std::min(std::max(v, 0.f), hi);
            }
            o[3] = c[3];
            if (pr) pr += 4;
        }
    });
}

void stepBloom(Image& state, const Image& cur, const MotionField& mf, const StepSettings& s,
               Image& scratch) {
    predict(state, mf, s.motionScale, s.smooth, scratch);
    const bool heal = s.heal > 0.f && cur.w == state.w && cur.h == state.h;
    parallelFor(state.h, [&](int y) {
        const float* p = scratch.row(y);
        const float* c = heal ? cur.row(y) : nullptr;
        float* o = state.row(y);
        for (int x = 0; x < state.w; ++x, p += 4, o += 4) {
            for (int k = 0; k < 4; ++k) {
                float v = p[k];
                if (c) v += (c[k] - v) * s.heal;
                o[k] = v;
            }
            if (c) c += 4;
        }
    });
}

uint64_t fingerprint(const Image& img) {
    uint64_t h = 1469598103934665603ull;
    auto mix = [&](uint32_t v) {
        h ^= v;
        h *= 1099511628211ull;
    };
    mix(uint32_t(img.w));
    mix(uint32_t(img.h));
    if (img.empty()) return h;
    for (int j = 0; j < 24; ++j) {
        int y = int((j + 0.5f) * img.h / 24);
        for (int i = 0; i < 40; ++i) {
            int x = int((i + 0.5f) * img.w / 40);
            const float* p = img.row(y) + x * 4;
            for (int k = 0; k < 4; ++k) {
                uint32_t bits;
                std::memcpy(&bits, p + k, 4);
                mix(bits);
            }
        }
    }
    return h;
}

}  // namespace mosh
