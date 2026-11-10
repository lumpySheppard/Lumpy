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

// ------------------------------------------------------------ randomness

inline uint64_t mix64(uint64_t x) {
    x += 0x9e3779b97f4a7c15ull;
    x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9ull;
    x = (x ^ (x >> 27)) * 0x94d049bb133111ebull;
    return x ^ (x >> 31);
}
inline uint64_t hashN(uint64_t a, uint64_t b, uint64_t c = 0, uint64_t d = 0, uint64_t e = 0) {
    return mix64(mix64(mix64(mix64(mix64(a) ^ b) ^ c) ^ d) ^ e);
}
// Uniform [0, 1).
inline float rnd(uint64_t a, uint64_t b, uint64_t c = 0, uint64_t d = 0, uint64_t e = 0) {
    return float(hashN(a, b, c, d, e) >> 40) * (1.f / 16777216.f);
}

// ------------------------------------------------------------ luma planes

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

// Uniform-grid block matching over a pyramid: full search at the coarsest
// level, +-1 refinement on the way down. Returns one vector per block.
void matchGrid(const std::vector<Plane>& pc, const std::vector<Plane>& pp, int block, int range,
               int bw, int bh, std::vector<int>& vx, std::vector<int>& vy) {
    int levels = 0;
    while (levels + 1 < int(pc.size()) && (block >> (levels + 1)) >= 4) ++levels;
    const int nb = bw * bh;
    vx.assign(nb, 0);
    vy.assign(nb, 0);

    for (int l = levels; l >= 0; --l) {
        const Plane& C = pc[l];
        const Plane& P = pp[l];
        const int bs = block >> l;
        // Penalise long vectors slightly, like a codec's rate term: static and
        // flat areas keep a zero vector instead of jittering.
        const float lambda = 0.004f * bs * bs;
        const bool coarsest = (l == levels);
        const int r = coarsest ? (range + (1 << l) - 1) >> l : 1;

        parallelFor(bh, [&](int by) {
            for (int bx = 0; bx < bw; ++bx) {
                int i = by * bw + bx;
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
                float area = float((x1 - x0) * (y1 - y0));
                if (coarsest && best < 0.002f * area) {  // static: skip search
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
}

// Summed-area tables of luma and luma^2, for block detail in O(1).
struct Integral {
    int w = 0, h = 0;
    std::vector<double> s, s2;  // (w+1) * (h+1)
    void build(const Plane& p) {
        w = p.w; h = p.h;
        s.assign(size_t(w + 1) * (h + 1), 0.0);
        s2.assign(size_t(w + 1) * (h + 1), 0.0);
        for (int y = 0; y < h; ++y) {
            double rs = 0, rs2 = 0;
            for (int x = 0; x < w; ++x) {
                double v = p.at(x, y);
                rs += v; rs2 += v * v;
                s[size_t(y + 1) * (w + 1) + x + 1] = s[size_t(y) * (w + 1) + x + 1] + rs;
                s2[size_t(y + 1) * (w + 1) + x + 1] = s2[size_t(y) * (w + 1) + x + 1] + rs2;
            }
        }
    }
    float stddev(int x0, int y0, int x1, int y1) const {
        x0 = std::max(0, x0); y0 = std::max(0, y0);
        x1 = std::min(w, x1); y1 = std::min(h, y1);
        if (x1 <= x0 || y1 <= y0) return 0.f;
        auto at = [&](const std::vector<double>& t, int x, int y) { return t[size_t(y) * (w + 1) + x]; };
        double n = double(x1 - x0) * (y1 - y0);
        double a = at(s, x1, y1) - at(s, x0, y1) - at(s, x1, y0) + at(s, x0, y0);
        double b = at(s2, x1, y1) - at(s2, x0, y1) - at(s2, x1, y0) + at(s2, x0, y0);
        double var = b / n - (a / n) * (a / n);
        return float(std::sqrt(std::max(0.0, var)));
    }
};

int pow2AtLeast4(int v) {
    int p = 4;
    while (p < v && p < 64) p *= 2;
    return p;
}

// Quadtree partition of a 64 px tile, mixing block sizes around `base`.
// Detailed areas tend to split smaller, flat areas tend to stay big, and
// `variety` adds randomness on top. Seed-only randomness keeps the layout
// stable from frame to frame (except where the picture's detail changes).
void partition(int x, int y, int s, int base, float variety, uint32_t seed, const Integral& ii,
               int w, int h, std::vector<Block>& out) {
    if (x >= w || y >= h) return;
    if (s > 4) {
        float detail = std::min(1.f, ii.stddev(x, y, x + s, y + s) / 0.08f);
        float u = rnd(seed, 0xB10C, uint64_t(x), uint64_t(y), uint64_t(s));
        bool split;
        if (s > base) split = !(u < variety * 0.65f * (1.f - detail));
        else split = u < variety * (0.15f + 0.6f * detail);
        if (split) {
            int hs = s / 2;
            partition(x, y, hs, base, variety, seed, ii, w, h, out);
            partition(x + hs, y, hs, base, variety, seed, ii, w, h, out);
            partition(x, y + hs, hs, base, variety, seed, ii, w, h, out);
            partition(x + hs, y + hs, hs, base, variety, seed, ii, w, h, out);
            return;
        }
    }
    out.push_back(Block{x, y, s});
}

}  // namespace

MotionField estimateMotion(const Image& cur, const Image& prev, const MotionSettings& s) {
    MotionField mf;
    if (cur.empty() || prev.w != cur.w || prev.h != cur.h) return mf;

    const int range = std::max(1, s.range);
    std::vector<Plane> pc{lumaOf(cur)}, pp{lumaOf(prev)};
    for (int l = 0; l < 3 && (pc.back().w >> 1) >= 32 && (pc.back().h >> 1) >= 32; ++l) {
        pc.push_back(half(pc.back()));
        pp.push_back(half(pp.back()));
    }
    mf.w = cur.w;
    mf.h = cur.h;

    if (s.variety <= 0.f) {
        // Fixed grid: one vector per block, the cell is the block.
        const int block = std::max(4, s.block);
        mf.cell = mf.base = block;
        mf.cw = (cur.w + block - 1) / block;
        mf.ch = (cur.h + block - 1) / block;
        std::vector<int> vx, vy;
        matchGrid(pc, pp, block, range, mf.cw, mf.ch, vx, vy);
        mf.dx.assign(vx.begin(), vx.end());
        mf.dy.assign(vy.begin(), vy.end());
        mf.blocks.reserve(size_t(mf.cw) * mf.ch);
        for (int by = 0; by < mf.ch; ++by)
            for (int bx = 0; bx < mf.cw; ++bx) mf.blocks.push_back(Block{bx * block, by * block, block});
        return mf;
    }

    // Mixed sizes: guide vectors on a 16 px grid, then each leaf block of the
    // quadtree picks the best of its neighbours' guides and refines it.
    const int base = pow2AtLeast4(s.block);
    const float variety = std::min(1.f, s.variety);
    const int G = 16;
    const int gw = (cur.w + G - 1) / G, gh = (cur.h + G - 1) / G;
    std::vector<int> gx, gy;
    matchGrid(pc, pp, G, range, gw, gh, gx, gy);

    Integral ii;
    ii.build(pc[0]);
    for (int ty = 0; ty < cur.h; ty += 64)
        for (int tx = 0; tx < cur.w; tx += 64)
            partition(tx, ty, 64, base, variety, s.seed, ii, cur.w, cur.h, mf.blocks);

    mf.cell = 4;
    mf.base = base;
    mf.cw = (cur.w + 3) / 4;
    mf.ch = (cur.h + 3) / 4;
    mf.dx.assign(size_t(mf.cw) * mf.ch, 0.f);
    mf.dy.assign(size_t(mf.cw) * mf.ch, 0.f);

    const Plane& C = pc[0];
    const Plane& P = pp[0];
    parallelFor(int(mf.blocks.size()), [&](int bi) {
        const Block& b = mf.blocks[bi];
        const int x0 = b.x, y0 = b.y;
        const int x1 = std::min(x0 + b.size, C.w), y1 = std::min(y0 + b.size, C.h);
        const float lambda = 0.004f * b.size * b.size;
        auto cost = [&](int ux, int uy, float limit) {
            float pen = lambda * float(std::abs(ux) + std::abs(uy));
            return blockSAD(C, P, x0, y0, x1, y1, ux, uy, limit - pen) + pen;
        };
        // Candidates: zero, plus guide vectors under and around the block.
        int cand[40][2];
        int nc = 0;
        cand[nc][0] = 0; cand[nc][1] = 0; ++nc;
        int gx0 = std::max(0, x0 / G - 1), gx1 = std::min(gw - 1, (x1 - 1) / G + 1);
        int gy0 = std::max(0, y0 / G - 1), gy1 = std::min(gh - 1, (y1 - 1) / G + 1);
        for (int y = gy0; y <= gy1 && nc < 40; ++y)
            for (int x = gx0; x <= gx1 && nc < 40; ++x) {
                int vx = gx[y * gw + x], vy = gy[y * gw + x];
                bool dup = false;
                for (int i = 0; i < nc && !dup; ++i) dup = cand[i][0] == vx && cand[i][1] == vy;
                if (!dup) { cand[nc][0] = vx; cand[nc][1] = vy; ++nc; }
            }
        int bx = 0, by = 0;
        float best = std::numeric_limits<float>::max();
        for (int i = 0; i < nc; ++i) {
            float c = cost(cand[i][0], cand[i][1], best);
            if (c < best) { best = c; bx = cand[i][0]; by = cand[i][1]; }
        }
        // Small diamond refinement.
        for (int it = 0; it < 4; ++it) {
            static const int d4[4][2] = {{1, 0}, {-1, 0}, {0, 1}, {0, -1}};
            bool moved = false;
            for (auto& d : d4) {
                int ux = bx + d[0], uy = by + d[1];
                if (std::abs(ux) > range || std::abs(uy) > range) continue;
                float c = cost(ux, uy, best);
                if (c < best) { best = c; bx = ux; by = uy; moved = true; }
            }
            if (!moved) break;
        }
        for (int cy = y0 / 4; cy < (y1 + 3) / 4; ++cy)
            for (int cx = x0 / 4; cx < (x1 + 3) / 4; ++cx) {
                mf.dx[size_t(cy) * mf.cw + cx] = float(bx);
                mf.dy[size_t(cy) * mf.cw + cx] = float(by);
            }
    });
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

// Box-blur a per-cell field so vectors can be interpolated smoothly.
std::vector<float> blurCells(const std::vector<float>& f, int cw, int ch, int r) {
    if (r <= 0) return f;
    std::vector<float> tmp(f.size()), out(f.size());
    for (int y = 0; y < ch; ++y)
        for (int x = 0; x < cw; ++x) {
            float s = 0; int n = 0;
            for (int i = std::max(0, x - r); i <= std::min(cw - 1, x + r); ++i, ++n) s += f[size_t(y) * cw + i];
            tmp[size_t(y) * cw + x] = s / n;
        }
    for (int y = 0; y < ch; ++y)
        for (int x = 0; x < cw; ++x) {
            float s = 0; int n = 0;
            for (int j = std::max(0, y - r); j <= std::min(ch - 1, y + r); ++j, ++n) s += tmp[size_t(j) * cw + x];
            out[size_t(y) * cw + x] = s / n;
        }
    return out;
}

// Per-cell behaviour for one step.
enum CellMode : uint8_t { kNormal = 0, kStuck = 1, kStreak = 2 };

struct Plan {
    MotionField mf;              // vectors after scatter
    std::vector<uint8_t> mode;   // per cell
    std::vector<float> anchor;   // streak source row, per cell
    int ecw = 0, ech = 0;             // colour error grid (4 px cells)
    std::vector<float> dY, dCb, dCr;  // colour error per 4 px cell (empty if none)
    std::vector<int> flatten;    // indices into mf.blocks
};

void predictPlan(const Image& ref, const Plan& pl, const std::vector<float>* sdx,
                 const std::vector<float>* sdy, float scale, Image& out) {
    const MotionField& mf = pl.mf;
    if (out.w != ref.w || out.h != ref.h) out = Image(ref.w, ref.h);
    if (mf.empty() || mf.w != ref.w || mf.h != ref.h) {
        out.px = ref.px;
        return;
    }
    const bool smooth = sdx && sdy;
    parallelFor(ref.h, [&](int y) {
        float* d = out.row(y);
        const int cy = std::min(y / mf.cell, mf.ch - 1);
        for (int x = 0; x < ref.w; ++x, d += 4) {
            const int cx = std::min(x / mf.cell, mf.cw - 1);
            const size_t c = size_t(cy) * mf.cw + cx;
            const uint8_t m = pl.mode.empty() ? uint8_t(kNormal) : pl.mode[c];
            if (m == kStuck) {
                std::memcpy(d, ref.row(y) + size_t(x) * 4, 4 * sizeof(float));
                continue;
            }
            if (m == kStreak) {
                sampleBilinear(ref, float(x), pl.anchor[c], d);
                continue;
            }
            float vx, vy;
            if (smooth) {
                float gx = (x + 0.5f) / mf.cell - 0.5f, gy = (y + 0.5f) / mf.cell - 0.5f;
                gx = std::min(std::max(gx, 0.f), float(mf.cw - 1));
                gy = std::min(std::max(gy, 0.f), float(mf.ch - 1));
                int x0 = int(gx), y0 = int(gy);
                int x1 = std::min(x0 + 1, mf.cw - 1), y1 = std::min(y0 + 1, mf.ch - 1);
                float tx = gx - x0, ty = gy - y0;
                auto lerp2 = [&](const std::vector<float>& f) {
                    float a = f[size_t(y0) * mf.cw + x0], b = f[size_t(y0) * mf.cw + x1];
                    float cc = f[size_t(y1) * mf.cw + x0], dd = f[size_t(y1) * mf.cw + x1];
                    float top = a + (b - a) * tx, bot = cc + (dd - cc) * tx;
                    return top + (bot - top) * ty;
                };
                vx = lerp2(*sdx);
                vy = lerp2(*sdy);
            } else {
                vx = mf.dx[c];
                vy = mf.dy[c];
            }
            sampleBilinear(ref, x + vx * scale, y + vy * scale, d);
        }
    });
}

// Glitch rolls are keyed by block position and a time bucket, so a glitch
// lasts `hold` frames and doesn't flicker; each block's bucket is offset so
// they don't all change on the same frame.
inline bool roll(uint32_t seed, const Block& b, int k, int hold, uint64_t kind, float chance,
                 uint64_t* bucketOut = nullptr) {
    if (chance <= 0.f) return false;
    hold = std::max(1, hold);
    uint64_t phase = hashN(seed, 0x9A5E, uint64_t(b.x), uint64_t(b.y)) % uint64_t(hold);
    uint64_t bucket = (uint64_t(int64_t(k) + (int64_t(1) << 40)) + phase) / uint64_t(hold);
    if (bucketOut) *bucketOut = bucket;
    return rnd(seed, kind, uint64_t(b.x) | (uint64_t(b.y) << 20) | (uint64_t(b.size) << 40), bucket) <
           chance;
}

Plan makePlan(const MotionField& src, const StepSettings& s, int k, int imgH) {
    Plan pl;
    pl.mf = src;
    const GlitchSettings& g = s.glitch;
    const bool anyBlock = g.stuck > 0 || g.scatter > 0 || g.streak > 0 || g.flatten > 0;
    const bool anyColour = g.colourErr > 0;
    if (!anyBlock && !anyColour) return pl;

    const size_t nc = size_t(src.cw) * src.ch;
    pl.mode.assign(nc, kNormal);
    pl.anchor.assign(nc, 0.f);
    if (anyColour) {
        pl.ecw = (src.w + 3) / 4;
        pl.ech = (imgH + 3) / 4;
        const size_t ne = size_t(pl.ecw) * pl.ech;
        pl.dY.assign(ne, 0.f);
        pl.dCb.assign(ne, 0.f);
        pl.dCr.assign(ne, 0.f);
    }
    const uint32_t seed = s.seed;
    for (size_t bi = 0; bi < src.blocks.size(); ++bi) {
        const Block& b = src.blocks[bi];
        const int cx0 = b.x / src.cell, cy0 = b.y / src.cell;
        const int cx1 = std::min(src.cw, (b.x + b.size + src.cell - 1) / src.cell);
        const int cy1 = std::min(src.ch, (b.y + b.size + src.cell - 1) / src.cell);
        auto forCells = [&](auto fn) {
            for (int cy = cy0; cy < cy1; ++cy)
                for (int cx = cx0; cx < cx1; ++cx) fn(size_t(cy) * src.cw + cx);
        };
        uint64_t bucket = 0;
        if (roll(seed, b, k, g.hold, 1, g.stuck)) {
            forCells([&](size_t c) { pl.mode[c] = kStuck; });
        } else if (roll(seed, b, k, g.hold, 2, g.streak)) {
            // Visual top row of the block; everything below copies it.
            float top = g.upSign > 0 ? float(std::min(b.y + b.size, imgH) - 1) : float(b.y);
            forCells([&](size_t c) { pl.mode[c] = kStreak; pl.anchor[c] = top; });
        } else if (roll(seed, b, k, g.hold, 3, g.scatter, &bucket)) {
            float r = float(std::max(8, s.range * 2));
            float vx = (rnd(seed, 30, uint64_t(b.x), uint64_t(b.y), bucket) * 2.f - 1.f) * r;
            float vy = (rnd(seed, 31, uint64_t(b.x), uint64_t(b.y), bucket) * 2.f - 1.f) * r;
            forCells([&](size_t c) { pl.mf.dx[c] = vx; pl.mf.dy[c] = vy; });
        }
        if (roll(seed, b, k, g.hold, 4, g.flatten)) pl.flatten.push_back(int(bi));
    }
    if (anyColour) {
        // Colour (DC) errors: a soft blotch of wrong colour lands on a single
        // frame; after that the motion carries it along, like a decoder
        // propagating a damaged block. Rare per macroblock, sized 1-3 blocks.
        const int E = std::max(16, src.base);
        const float st = g.colourErrStrength * 0.35f;
        const float chance = std::min(1.f, g.colourErr) * 0.012f;
        const uint64_t kk = uint64_t(int64_t(k) + (int64_t(1) << 40));
        for (int ey = 0; ey < imgH; ey += E)
            for (int ex = 0; ex < src.w; ex += E) {
                const uint64_t key = (uint64_t(ex) << 20) ^ uint64_t(ey);
                if (rnd(seed, 50, key, kk) >= chance) continue;
                const float R = E * (0.6f + 1.2f * rnd(seed, 54, key, kk));
                const float cxp = ex + E * rnd(seed, 55, key, kk), cyp = ey + E * rnd(seed, 56, key, kk);
                const float ang = 6.2831853f * rnd(seed, 51, key, kk);
                const float mag = st * (0.5f + 0.5f * rnd(seed, 52, key, kk));
                const float db = std::cos(ang) * mag, dr = std::sin(ang) * mag;
                const float dy = (rnd(seed, 53, key, kk) * 2.f - 1.f) * st * 0.3f;
                const int gx0 = std::max(0, int((cxp - R) / 4)), gx1 = std::min(pl.ecw - 1, int((cxp + R) / 4));
                const int gy0 = std::max(0, int((cyp - R) / 4)), gy1 = std::min(pl.ech - 1, int((cyp + R) / 4));
                for (int gy = gy0; gy <= gy1; ++gy)
                    for (int gx = gx0; gx <= gx1; ++gx) {
                        float d = std::hypot(gx * 4 + 2 - cxp, gy * 4 + 2 - cyp) / R;
                        if (d >= 1.f) continue;
                        float wgt = (1.f - d * d) * (1.f - d * d);
                        size_t c = size_t(gy) * pl.ecw + gx;
                        pl.dY[c] += dy * wgt; pl.dCb[c] += db * wgt; pl.dCr[c] += dr * wgt;
                    }
            }
    }
    return pl;
}

float maxValue(const Image& img) {
    float m = 1.f;
    for (size_t i = 0; i < img.px.size(); i += 4)
        m = std::max(m, std::max(img.px[i], std::max(img.px[i + 1], img.px[i + 2])));
    return m;
}

// Rec.709 Y'CbCr.
inline void toYCC(const float* p, float& y, float& cb, float& cr) {
    y = 0.2126f * p[0] + 0.7152f * p[1] + 0.0722f * p[2];
    cb = (p[2] - y) / 1.8556f;
    cr = (p[0] - y) / 1.5748f;
}
inline void fromYCC(float y, float cb, float cr, float* p) {
    float r = y + 1.5748f * cr;
    float b = y + 1.8556f * cb;
    p[0] = r;
    p[2] = b;
    p[1] = (y - 0.2126f * r - 0.0722f * b) / 0.7152f;
}

}  // namespace

void predict(const Image& ref, const MotionField& mf, float scale, bool smooth, Image& out) {
    Plan pl;
    pl.mf = mf;
    std::vector<float> sdx, sdy;
    if (smooth && !mf.empty()) {
        int r = std::max(0, mf.base / mf.cell / 2);
        sdx = blurCells(mf.dx, mf.cw, mf.ch, r);
        sdy = blurCells(mf.dy, mf.cw, mf.ch, r);
    }
    predictPlan(ref, pl, smooth ? &sdx : nullptr, smooth ? &sdy : nullptr, scale, out);
}

std::vector<StepSpec> schedule(int start, int t, const TimeSettings& ts) {
    std::vector<StepSpec> out;
    if (t <= start) return out;
    out.reserve(size_t(t - start));
    enum { kRun, kRepeat, kFreeze, kReverse };
    int p = start, remaining = 0, mode = kRun;
    const float ch[5] = {std::max(0.f, ts.repeat), std::max(0.f, ts.freeze),
                         std::max(0.f, ts.jumpBack), std::max(0.f, ts.jumpFwd),
                         std::max(0.f, ts.reverse)};
    const float sum = ch[0] + ch[1] + ch[2] + ch[3] + ch[4];
    const float norm = sum > 1.f ? 1.f / sum : 1.f;
    const int maxLen = std::max(1, ts.maxLen), maxJump = std::max(1, ts.maxJump);

    for (int k = start + 1; k <= t; ++k) {
        if (remaining <= 0) {
            mode = kRun;
            const bool allowed = ts.every <= 0 || ((k - start) % ts.every) == 0;
            if (allowed && sum > 0.f) {
                float r = rnd(ts.seed, 0x71AE, uint64_t(k));
                int ev = -1;
                float acc = 0.f;
                for (int i = 0; i < 5; ++i) {
                    acc += ch[i] * norm;
                    if (r < acc) { ev = i; break; }
                }
                int len = 1 + std::min(maxLen - 1, int(rnd(ts.seed, 0x71AF, uint64_t(k)) * maxLen));
                int dist = 1 + std::min(maxJump - 1, int(rnd(ts.seed, 0x71B0, uint64_t(k)) * maxJump));
                if (ev == 0) { mode = kRepeat; remaining = len; }
                else if (ev == 1) { mode = kFreeze; remaining = len; }
                else if (ev == 2) { p = std::max(start, p - dist); }
                else if (ev == 3) {
                    // Lurch: one step carrying the motion of the whole jump.
                    out.push_back(StepSpec{true, p + dist, p});
                    p += dist;
                    continue;
                } else if (ev == 4) { mode = kReverse; remaining = len; }
            }
        }
        StepSpec s;
        switch (mode) {
            case kRepeat: {
                int a = std::max(p, start + 1);
                s = StepSpec{true, a, a - 1};
                break;
            }
            case kFreeze:
                s = StepSpec{false, 0, 0};
                break;
            case kReverse:
                if (p - 1 >= start) {
                    s = StepSpec{true, p - 1, p};
                    --p;
                    break;
                }
                mode = kRun;
                remaining = 0;
                [[fallthrough]];
            default:
                s = StepSpec{true, p + 1, p};
                ++p;
        }
        out.push_back(s);
        --remaining;
    }
    return out;
}

void step(Image& state, const Image* resCur, const Image* resPrev, const Image* healTo,
          const MotionField& mfIn, const StepSettings& s, int k, Scratch& sc) {
    if (state.empty()) return;
    Plan pl = makePlan(mfIn, s, k, state.h);

    std::vector<float> sdx, sdy;
    if (s.smooth && !pl.mf.empty()) {
        int r = std::max(0, pl.mf.base / pl.mf.cell / 2);
        sdx = blurCells(pl.mf.dx, pl.mf.cw, pl.mf.ch, r);
        sdy = blurCells(pl.mf.dy, pl.mf.cw, pl.mf.ch, r);
    }
    const std::vector<float>* px = s.smooth ? &sdx : nullptr;
    const std::vector<float>* py = s.smooth ? &sdy : nullptr;

    predictPlan(state, pl, px, py, s.motionScale, sc.predState);
    const ColourSettings& col = s.colour;
    const bool drift = col.chromaDrift > 0.f;
    if (drift) predictPlan(state, pl, px, py, s.motionScale * (1.f + 2.f * col.chromaDrift), sc.predDrift);

    const bool useRes = s.residual > 0.f && resCur && resPrev && resCur->w == state.w &&
                        resCur->h == state.h && resPrev->w == state.w && resPrev->h == state.h;
    if (useRes) predictPlan(*resPrev, pl, px, py, s.motionScale, sc.predReal);
    const bool heal = s.heal > 0.f && healTo && healTo->w == state.w && healTo->h == state.h;
    const float hi = maxValue(useRes ? *resCur : (heal ? *healTo : state));

    const float chromaGain = 1.f + 1.5f * col.bleed;
    const float sat = 1.f + 0.025f * col.satBuild;
    const float ang = col.hueDrift * 3.14159265f / 180.f;
    const float ca = std::cos(ang) * sat, sa = std::sin(ang) * sat;
    const bool rotate = col.hueDrift != 0.f || col.satBuild != 0.f;
    const bool errs = !pl.dY.empty();
    const bool plainRGB = !drift && !rotate && !errs && col.bleed == 0.f;
    const MotionField& mf = pl.mf;

    parallelFor(state.h, [&](int y) {
        const float* ps = sc.predState.row(y);
        const float* pd = drift ? sc.predDrift.row(y) : nullptr;
        const float* pr = useRes ? sc.predReal.row(y) : nullptr;
        const float* rc = useRes ? resCur->row(y) : nullptr;
        const float* hc = heal ? healTo->row(y) : nullptr;
        float* o = state.row(y);
        const int cy = std::min(y / mf.cell, std::max(0, mf.ch - 1));
        for (int x = 0; x < state.w; ++x) {
            const size_t i = size_t(x) * 4;
            const size_t c = mf.empty() ? 0 : size_t(cy) * mf.cw + std::min(x / mf.cell, mf.cw - 1);
            const bool stuck = !pl.mode.empty() && pl.mode[c] == kStuck;
            if (plainRGB) {
                // No colour controls: same maths in RGB, much cheaper.
                for (int ch = 0; ch < 3; ++ch) {
                    float v = ps[i + ch];
                    if (pr && !stuck) v += s.residual * (rc[i + ch] - pr[i + ch]);
                    if (hc) v += (hc[i + ch] - v) * s.heal;
                    o[i + ch] = std::min(std::max(v, 0.f), hi);
                }
                o[i + 3] = rc ? rc[i + 3] : ps[i + 3];
                continue;
            }
            float Y, Cb, Cr;
            toYCC(ps + i, Y, Cb, Cr);
            if (pd) { float y2; toYCC(pd + i, y2, Cb, Cr); }
            if (pr && !stuck) {
                float y1, b1, r1, y0, b0, r0;
                toYCC(rc + i, y1, b1, r1);
                toYCC(pr + i, y0, b0, r0);
                Y += s.residual * (y1 - y0);
                Cb += s.residual * chromaGain * (b1 - b0);
                Cr += s.residual * chromaGain * (r1 - r0);
            }
            if (rotate) {
                float nb = Cb * ca - Cr * sa, nr = Cb * sa + Cr * ca;
                Cb = nb; Cr = nr;
            }
            if (errs) {
                const size_t e = size_t(std::min(y / 4, pl.ech - 1)) * pl.ecw + std::min(x / 4, pl.ecw - 1);
                Y += pl.dY[e]; Cb += pl.dCb[e]; Cr += pl.dCr[e];
            }
            float rgb[3];
            fromYCC(Y, Cb, Cr, rgb);
            for (int ch = 0; ch < 3; ++ch) {
                float v = rgb[ch];
                if (hc) v += (hc[i + ch] - v) * s.heal;
                o[i + ch] = std::min(std::max(v, 0.f), hi);
            }
            o[i + 3] = rc ? rc[i + 3] : ps[i + 3];
        }
    });

    // Flattened blocks: one flat colour each (a block that lost its AC data).
    parallelFor(int(pl.flatten.size()), [&](int fi) {
        const Block& b = mf.blocks[pl.flatten[fi]];
        const int x1 = std::min(b.x + b.size, state.w), y1 = std::min(b.y + b.size, state.h);
        double sum[3] = {0, 0, 0};
        int n = 0;
        for (int y = b.y; y < y1; ++y)
            for (int x = b.x; x < x1; ++x, ++n) {
                const float* p = state.row(y) + size_t(x) * 4;
                sum[0] += p[0]; sum[1] += p[1]; sum[2] += p[2];
            }
        if (!n) return;
        for (int y = b.y; y < y1; ++y)
            for (int x = b.x; x < x1; ++x) {
                float* p = state.row(y) + size_t(x) * 4;
                p[0] = float(sum[0] / n); p[1] = float(sum[1] / n); p[2] = float(sum[2] / n);
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
