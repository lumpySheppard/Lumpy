#include "GateEngine.h"

#include <algorithm>
#include <atomic>
#include <cmath>
#include <cstdint>
#include <thread>

#include "Encodings.h"

namespace gate {

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

inline float smoothstep(float a, float b, float x) {
    if (b <= a) return x >= a ? 1.f : 0.f;
    float t = std::min(std::max((x - a) / (b - a), 0.f), 1.f);
    return t * t * (3.f - 2.f * t);
}

// Gaussian-ish blur of an RGB profile (3 box passes), clamped at the ends.
std::vector<float> blurProfile(const std::vector<float>& p, int n, float radius) {
    std::vector<float> a = p, b(p.size());
    const int r = std::max(0, int(std::lround(radius / 1.7f)));
    if (r == 0) return a;
    for (int pass = 0; pass < 3; ++pass) {
        for (int c = 0; c < 3; ++c) {
            float acc = 0;
            for (int i = -r; i <= r; ++i) acc += a[size_t(std::min(std::max(i, 0), n - 1)) * 3 + c];
            for (int x = 0; x < n; ++x) {
                b[size_t(x) * 3 + c] = acc / float(2 * r + 1);
                acc += a[size_t(std::min(x + r + 1, n - 1)) * 3 + c] - a[size_t(std::max(x - r, 0)) * 3 + c];
            }
        }
        std::swap(a, b);
    }
    return a;
}

// Running min (or max) over a (2r+1) window along rows, then columns (a
// square window). Monotonic deque, O(1) per pixel.
void slidingExtreme(std::vector<float>& p, int w, int h, int r, bool takeMax) {
    auto better = [takeMax](float a, float b) { return takeMax ? a >= b : a <= b; };
    auto run = [&](int n, auto get, auto set) {
        std::vector<int> dq(size_t(n) + 1);
        std::vector<float> out(n);
        int head = 0, tail = 0, next = 0;
        for (int i = 0; i < n; ++i) {
            const int hi = std::min(n - 1, i + r);
            for (; next <= hi; ++next) {
                while (tail > head && better(get(next), get(dq[tail - 1]))) --tail;
                dq[tail++] = next;
            }
            while (dq[head] < i - r) ++head;
            out[i] = get(dq[head]);
        }
        for (int i = 0; i < n; ++i) set(i, out[i]);
    };
    parallelFor(h, [&](int y) {
        float* row = &p[size_t(y) * w];
        run(w, [&](int i) { return row[i]; }, [&](int i, float v) { row[i] = v; });
    });
    parallelFor(w, [&](int x) {
        run(h, [&](int i) { return p[size_t(i) * w + x]; }, [&](int i, float v) { p[size_t(i) * w + x] = v; });
    });
}

// Per-channel blur radii (for colour fringing).
std::vector<float> blurChannels(const std::vector<float>& p, int n, const float r[3]) {
    std::vector<float> out(p.size());
    for (int c = 0; c < 3; ++c) {
        const std::vector<float> b = blurProfile(p, n, r[c]);
        for (int u = 0; u < n; ++u) out[size_t(u) * 3 + c] = b[size_t(u) * 3 + c];
    }
    return out;
}

inline uint32_t hash3(int32_t x, int32_t y, uint32_t seed) {
    uint32_t h = uint32_t(x) * 0x8da6b343u ^ uint32_t(y) * 0xd8163841u ^ seed * 0xcb1ab31fu;
    h ^= h >> 13;
    h *= 0x5bd1e995u;
    h ^= h >> 15;
    return h;
}

// Smooth 2D value noise in [0, 1].
float valueNoise2(float x, float y, uint32_t seed) {
    const float fx = std::floor(x), fy = std::floor(y);
    const int32_t ix = int32_t(fx), iy = int32_t(fy);
    float tx = x - fx, ty = y - fy;
    tx = tx * tx * (3.f - 2.f * tx);
    ty = ty * ty * (3.f - 2.f * ty);
    auto r = [&](int32_t a, int32_t b) { return float(hash3(a, b, seed) >> 8) / 16777216.f; };
    const float a = r(ix, iy), b = r(ix + 1, iy), c = r(ix, iy + 1), d = r(ix + 1, iy + 1);
    const float top = a + (b - a) * tx, bot = c + (d - c) * tx;
    return top + (bot - top) * ty;
}

enum Edge { kTop, kBottom, kLeft, kRight };

}  // namespace

void render(const Image& in, const Settings& s, Image& out) {
    const int w = in.w, h = in.h;
    if (in.empty()) {
        out = in;
        return;
    }
    const bool scene = encodings::isSceneReferred(s.encoding);
    const auto vis = [&](int row) { return s.upSign > 0 ? h - 1 - row : row; };  // row -> visual y (0 = top)

    // Linear light, and the brightness (as encoded) of each pixel.
    std::vector<float> L(size_t(w) * h * 3), S(size_t(w) * h * 3), V(size_t(w) * h);
    parallelFor(h, [&](int y) {
        const float* p = in.row(y);
        const int yv = vis(y);
        for (int x = 0; x < w; ++x) {
            float l[3];
            for (int c = 0; c < 3; ++c) l[c] = encodings::decode(p[x * 4 + c], s.encoding);
            const float lum = 0.2126f * l[0] + 0.7152f * l[1] + 0.0722f * l[2];
            const size_t i = (size_t(yv) * w + x) * 3;  // stored in visual order
            for (int c = 0; c < 3; ++c) L[i + c] = l[c];
            V[size_t(yv) * w + x] = encodings::encode(std::max(lum, 0.f), s.encoding);
        }
    });
    // Isolate Lights: a morphological top-hat. Opening (min filter, then max
    // filter, both wider than a light) removes every bright feature smaller
    // than a light and keeps big ones, walls included, right up to their
    // edges. What the opening removed is what counts as a light.
    std::vector<float> opened;
    if (s.isolate > 0.f) {
        opened = V;
        const int r = std::max(1, int(std::lround(s.lightSize * h / 200.f)));
        slidingExtreme(opened, w, h, r, false);
        slidingExtreme(opened, w, h, r, true);
    }
    std::vector<float> Wt(size_t(w) * h);  // how much each pixel counts as a source
    parallelFor(h, [&](int yv) {
        for (int x = 0; x < w; ++x) {
            const size_t j = size_t(yv) * w + x;
            float k = smoothstep(s.threshold - s.knee, s.threshold + s.knee, V[j]);
            if (!opened.empty()) {
                const float tophat = V[j] - opened[j];  // how much it stands out as a small light
                k *= 1.f - s.isolate + s.isolate * smoothstep(0.03f, 0.15f, tophat);
            }
            Wt[j] = k;
            // Clipped lights have lost most of their real brightness: give a
            // source about 6x, tapering off for genuinely bright scene-linear
            // lights that still have theirs.
            const float lum = 0.2126f * L[j * 3] + 0.7152f * L[j * 3 + 1] + 0.0722f * L[j * 3 + 2];
            const float boost = 6.f / (1.f + std::max(lum, 0.f) / 16.f);
            for (int c = 0; c < 3; ++c) S[j * 3 + c] = std::max(L[j * 3 + c], 0.f) * k * boost;
        }
    });

    // The gate, in visual pixel coordinates.
    const float f = std::min(std::max(s.crop, 0.f), 60.f) / 100.f;
    const float gx0 = w * f / 2, gx1 = w - gx0, gy0 = h * f / 2, gy1 = h - gy0;
    const float unit = h / 100.f;  // percentages of frame height
    const float reachPx = std::max(1.f, s.reach * unit);
    const float startPx = s.flareStart * unit;          // + inside the frame, - beyond the edge
    const float rampPx = std::max(1.f, 0.2f * reachPx);  // how quickly it comes up once it starts
    const float depthPx = std::max(1.f, s.depth * unit);
    const float spreadPx = std::max(0.f, s.spread * unit);

    // Per edge: profile along it, at several blur widths (the plume fans out).
    const int K = 6;
    const float bladeTan = std::tan(std::min(std::max(s.bladeAngle, -75.f), 75.f) * 3.14159265f / 180.f);
    const float bladeHw0 = std::max(1.f, spreadPx * 0.5f);
    // The flare is computed out to 5x Depth (exp(-5) < 1%) and faded to zero
    // over the last stretch, so it never ends in a visible line.
    const float maxD = depthPx * 5.f;
    // Fringe: red scatters a little wider than blue.
    const float fr = std::min(std::max(s.fringe, 0.f), 2.f);
    const float chanScale[3] = {1.f + 0.5f * fr, 1.f, std::max(0.2f, 1.f - 0.3f * fr)};
    // Streaks: a texture along the edge (the gate's nicks and roughness), which
    // shimmers over time. Spacing from 5% of the height (broad) to 0.5% (fine).
    const float spacing = std::max(1.f, h * 0.05f * std::pow(0.1f, std::min(std::max(s.streakDetail, 0.f), 1.f)));
    const float tShimmer = s.time * s.shimmerSpeed * 0.15f;
    const float flicker = 1.f + 0.25f * std::min(std::max(s.shimmer, 0.f), 1.f) *
                                    (2.f * valueNoise2(s.time * s.shimmerSpeed * 0.5f, 0.f, s.seed + 99u) - 1.f);
    struct EdgeData {
        bool on = false;
        int n = 0;
        std::vector<std::vector<float>> levels;   // K profiles (plume / beam)
        std::vector<std::vector<float>> streak;   // K streak textures, softening with depth
        std::vector<float> glow;
        std::vector<float> cum;   // prefix sums of the (softened) profile, (n+1)*3 (blade)
    };
    EdgeData ed[4];
    for (int e = 0; e < 4; ++e) {
        if (!s.edges[e]) continue;
        const bool horiz = e == kTop || e == kBottom;
        const int n = horiz ? w : h;
        const float edgePos = e == kTop ? gy0 : e == kBottom ? gy1 : e == kLeft ? gx0 : gx1;
        const int inward = (e == kTop || e == kLeft) ? 1 : -1;  // +1: frame is at larger coordinates
        const int span = horiz ? h : w;
        std::vector<float> prof(size_t(n) * 3, 0.f);
        // A light starts to flare once it's within Flare Start of the edge
        // (positive: still inside the frame; negative: already past it), ramps
        // up over a short distance, and fades out over Reach beyond the edge.
        const int v0 = std::min(-int(std::ceil(reachPx * 1.5f)), int(std::floor(startPx - rampPx)) - 1);
        const int v1 = std::max(0, int(std::ceil(startPx)) + 1);
        parallelFor(n, [&](int u) {
            float acc[3] = {0, 0, 0};
            for (int v = v0; v <= v1; ++v) {
                const int q = int(std::floor(edgePos + float(v * inward)));
                if (q < 0 || q >= span) continue;
                const float fv = float(v);
                const float onset = 1.f - smoothstep(startPx - rampPx, startPx, fv);
                const float beyond = std::min(fv, 0.f) - std::min(startPx - rampPx, 0.f);  // distance past where it peaks
                const float wgt = onset * std::exp(std::min(beyond, 0.f) / (0.5f * reachPx));
                const size_t i = horiz ? (size_t(q) * w + u) * 3 : (size_t(u) * w + q) * 3;
                for (int c = 0; c < 3; ++c) acc[c] += S[i + c] * wgt;
            }
            for (int c = 0; c < 3; ++c) prof[size_t(u) * 3 + c] = acc[c] / (0.5f * reachPx);
        });
        ed[e].on = true;
        ed[e].n = n;
        const float fan = s.shape == kBeam ? 0.f : s.fan;  // a beam doesn't widen
        if (s.shape == kPlume || s.shape == kBeam) {
            for (int k = 0; k < K; ++k) {
                const float d = maxD * k / (K - 1);
                const float r0 = spreadPx + fan * d;
                const float r[3] = {r0 * chanScale[0], r0 * chanScale[1], r0 * chanScale[2]};
                ed[e].levels.push_back(blurChannels(prof, n, r));
            }
        }
        if (s.shape == kBlade) {
            const std::vector<float> soft = blurProfile(prof, n, std::max(0.f, s.bladeSoftness * unit));
            ed[e].cum.assign(size_t(n + 1) * 3, 0.f);
            for (int u = 0; u < n; ++u)
                for (int c = 0; c < 3; ++c)
                    ed[e].cum[size_t(u + 1) * 3 + c] = ed[e].cum[size_t(u) * 3 + c] + soft[size_t(u) * 3 + c];
        }
        if (s.streaks > 0.f) {
            // 3 octaves of value noise along the edge; a static part (the gate)
            // blended with a moving part (shimmer).
            const float sh = std::min(std::max(s.shimmer, 0.f), 1.f);
            std::vector<float> tex(size_t(n) * 3);
            for (int u = 0; u < n; ++u) {
                float acc = 0.f, norm = 0.f;
                for (int o = 0; o < 3; ++o) {
                    const float f = float(1 << o), a = 1.f / f;
                    const float x = (u + 1000.f * e) / spacing * f;
                    const float st = valueNoise2(x, 0.f, s.seed + 17u * o);
                    const float mv = valueNoise2(x * 1.31f + 57.f, tShimmer * f, s.seed + 101u + 17u * o);
                    acc += a * (st + (mv - st) * sh);
                    norm += a;
                }
                tex[size_t(u) * 3] = tex[size_t(u) * 3 + 1] = tex[size_t(u) * 3 + 2] = acc / norm;
            }
            for (int k = 0; k < K; ++k) {
                // Streaks blur out more slowly than the flare widens.
                const float d = maxD * k / (K - 1);
                ed[e].streak.push_back(blurProfile(tex, n, 0.35f * fan * d));
            }
        }
        ed[e].glow = blurProfile(prof, n, std::max(1.f, spreadPx * 0.4f));
    }

    // Composite in linear light. Film: scene-referred footage tops out around
    // 6 stops over mid grey, like FilmFlash's negative.
    const float filmW = 0.18f * 64.f;
    out = Image(w, h);
    const float glowFall = 0.004f * h + 1.f;
    Image full(w, h);
    parallelFor(h, [&](int y) {
        const int yv = vis(y);
        float* d = full.row(y);
        const float* src = in.row(y);
        for (int x = 0; x < w; ++x) {
            const size_t i = (size_t(yv) * w + x) * 3;
            const bool inside = x >= gx0 && x < gx1 && yv >= gy0 && yv < gy1;
            float add[3] = {0, 0, 0};
            if (inside) {
                for (int e = 0; e < 4; ++e) {
                    if (!ed[e].on) continue;
                    const float v = e == kTop ? yv - gy0 : e == kBottom ? gy1 - 1 - yv : e == kLeft ? x - gx0 : gx1 - 1 - x;
                    if (v > maxD) continue;
                    const int u = (e == kTop || e == kBottom) ? x : yv;
                    const float fade = 1.f - smoothstep(0.6f * maxD, maxD, v);
                    const float glow = std::exp(-v / glowFall) * s.edgeGlow;
                    const float* g = &ed[e].glow[size_t(u) * 3];
                    for (int c = 0; c < 3; ++c) add[c] += g[c] * glow;
                    // Falloff into the frame: a hot, tight core at the edge plus a
                    // long subtle tail.
                    const float core = std::min(std::max(s.core, 0.f), 1.f);
                    const float fall = ((1.f - core) * std::exp(-v / depthPx) +
                                        core * 2.2f * std::exp(-v / (0.2f * depthPx))) * s.intensity * fade * flicker;
                    const float fk = v / maxD * (K - 1);
                    const int k0 = std::min(int(fk), K - 2);
                    const float t = fk - k0;
                    // Streaks, following the flare's direction (a blade leans).
                    float mod = 1.f;
                    if (!ed[e].streak.empty()) {
                        const int n = ed[e].n;
                        const float uu = s.shape == kBlade ? u - v * bladeTan : float(u);
                        const int ui = std::min(std::max(int(std::lround(uu)), 0), n - 1);
                        const float na = ed[e].streak[k0][size_t(ui) * 3], nb = ed[e].streak[k0 + 1][size_t(ui) * 3];
                        mod = std::max(0.f, 1.f + 2.f * s.streaks * ((na + (nb - na) * t) - 0.5f));
                    }
                    if (s.shape == kPlume || s.shape == kBeam) {
                        const float* a = &ed[e].levels[k0][size_t(u) * 3];
                        const float* b = &ed[e].levels[k0 + 1][size_t(u) * 3];
                        for (int c = 0; c < 3; ++c) add[c] += (a[c] + (b[c] - a[c]) * t) * fall * mod;
                    } else {
                        // Blade: a box along the edge, widening with Fan and leaning
                        // with Angle: uniform inside, crisp (Blade Softness) at the sides.
                        const int n = ed[e].n;
                        const float hw = bladeHw0 + s.fan * 0.5f * v;
                        const float centre = u - v * bladeTan;
                        auto cumAt = [&](float q, int c) {
                            q = std::min(std::max(q, 0.f), float(n));
                            const int q0 = std::min(int(q), n - 1);
                            const float tq = q - q0;
                            const float a0 = ed[e].cum[size_t(q0) * 3 + c], a1 = ed[e].cum[size_t(q0 + 1) * 3 + c];
                            return a0 + (a1 - a0) * tq;
                        };
                        const float amp = fall * mod / (2.f * bladeHw0) * std::sqrt(bladeHw0 / hw);
                        for (int c = 0; c < 3; ++c) {
                            // Fringe: red's box a little wider than blue's.
                            const float hwc = hw * chanScale[c];
                            add[c] += (cumAt(centre + hwc + 0.5f, c) - cumAt(centre - hwc + 0.5f, c)) * amp / chanScale[c];
                        }
                    }
                }
                // Take the sources' colour, or go white, then tint.
                const float lum = 0.2126f * add[0] + 0.7152f * add[1] + 0.0722f * add[2];
                for (int c = 0; c < 3; ++c) add[c] = (lum + (add[c] - lum) * s.sourceColour) * s.tint[c];
            }
            for (int c = 0; c < 3; ++c) {
                float l = L[i + c];
                const float a = std::max(add[c], 0.f);
                if (s.composite == kFilm) {
                    // The flare is extra exposure on the negative: added in odds
                    // space and brought back through a soft shoulder, so it glows
                    // into the highlights and never flattens out into clipped white.
                    if (scene) {
                        const float o = std::max(l, 0.f) / filmW, o2 = o + a / filmW;
                        l = filmW * (1.f + o) * o2 / (1.f + o2);
                    } else {
                        const float tv = std::min(std::max(l, 0.f), 0.99995f);
                        const float o2 = tv / (1.f - tv) + a;
                        l = o2 / (1.f + o2);
                    }
                } else if (s.composite == kScreen && !scene) {
                    l = 1.f - (1.f - std::min(std::max(l, 0.f), 1.f)) * (1.f - std::min(a, 1.f));
                } else {
                    l += a;  // Add (and Screen on scene-referred footage)
                }
                if (s.showGate) {
                    if (!inside) l *= 0.35f;
                    // Sources in magenta, as strongly as they count.
                    const float kk = Wt[size_t(yv) * w + x];
                    const float mag = c == 1 ? 0.05f : 0.9f;
                    l += (mag - l) * std::min(kk * 1.2f, 1.f);
                    const bool border = std::fabs(x - gx0) < 1.5f || std::fabs(x - gx1) < 1.5f ||
                                        std::fabs(yv - gy0) < 1.5f || std::fabs(yv - gy1) < 1.5f;
                    if (border) l = c == 0 ? 1.f : 0.f;
                }
                d[x * 4 + c] = encodings::encode(l, s.encoding);
            }
            d[x * 4 + 3] = src[x * 4 + 3];
        }
    });

    if (!s.cropOutput || s.showGate || f <= 0.f) {
        out = std::move(full);
        return;
    }
    // Scale the gate up to fill the frame (bilinear).
    const float sx = (gx1 - gx0) / w, sy = (gy1 - gy0) / h;
    parallelFor(h, [&](int y) {
        float* d = out.row(y);
        const float fy = std::min(std::max(gy0 + (y + 0.5f) * sy - 0.5f, 0.f), float(h - 1));
        const int y0 = int(fy), y1 = std::min(y0 + 1, h - 1);
        const float ty = fy - y0;
        for (int x = 0; x < w; ++x) {
            const float fx = std::min(std::max(gx0 + (x + 0.5f) * sx - 0.5f, 0.f), float(w - 1));
            const int x0 = int(fx), x1 = std::min(x0 + 1, w - 1);
            const float tx = fx - x0;
            const float* a = full.row(y0) + x0 * 4;
            const float* b = full.row(y0) + x1 * 4;
            const float* c = full.row(y1) + x0 * 4;
            const float* e = full.row(y1) + x1 * 4;
            for (int k = 0; k < 4; ++k) {
                float top = a[k] + (b[k] - a[k]) * tx, bot = c[k] + (e[k] - c[k]) * tx;
                d[x * 4 + k] = top + (bot - top) * ty;
            }
        }
    });
}

}  // namespace gate
