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

// ------------------------------------------------------------ one edge's flare

// Single-channel blur (3 box passes), clamped at the ends.
std::vector<float> blur1(const std::vector<float>& p, int n, float radius) {
    std::vector<float> a = p, b(p.size());
    const int r = std::max(0, int(std::lround(radius / 1.7f)));
    if (r == 0) return a;
    for (int pass = 0; pass < 3; ++pass) {
        float acc = 0;
        for (int i = -r; i <= r; ++i) acc += a[size_t(std::min(std::max(i, 0), n - 1))];
        for (int x = 0; x < n; ++x) {
            b[size_t(x)] = acc / float(2 * r + 1);
            acc += a[size_t(std::min(x + r + 1, n - 1))] - a[size_t(std::max(x - r, 0))];
        }
        std::swap(a, b);
    }
    return a;
}

struct FlareGeom {
    int e = 0, n = 0, span = 0, inward = 1, w = 0, h = 0;
    float edgePos = 0, unit = 1;
};

const int K = 6;  // depth levels (the flare widens and softens with depth)

struct Flare {
    Look L;
    float weight = 1.f;
    int n = 0;
    float depthPx = 1, spreadPx = 0, maxD = 1, bladeTan = 0, bladeHw0 = 1, glowFall = 1, flicker = 1;
    float chanScale[3] = {1, 1, 1};
    std::vector<std::vector<float>> levels;   // RGB profile per depth level (plume / beam)
    std::vector<std::vector<float>> den, num; // luminance and luminance*position, per level: where the light is
    std::vector<std::vector<float>> streak;   // streak texture per level
    std::vector<float> glow;                  // RGB
    std::vector<float> cum;                   // RGB prefix sums (blade)
    // Pinch: each light collapsed to where it is and how much light it has.
    struct Src { float cen; float e[3]; float exit; };
    std::vector<Src> srcs;
    float pinchPx = 0, leanTan = 0, neckHw = 1, farFall = 1;
    std::vector<float> rays;                  // ray texture across the fan, rho in [-2, 2]
    std::vector<float> veil;                  // RGB, the edge profile blurred very wide
    std::vector<float> exitProf;              // how far out of frame the light feeding each point is (0..1)
    float veilFall = 1, frameH = 100, time = 0;
    unsigned seed = 1;
    static constexpr int kRays = 512;

    static float at1(const std::vector<float>& a, float q, int n) {
        q = std::min(std::max(q, 0.f), float(n - 1));
        const int q0 = std::min(int(q), n - 2 < 0 ? 0 : n - 2);
        const float t = std::min(q - q0, 1.f);
        return n > 1 ? a[size_t(q0)] + (a[size_t(q0 + 1)] - a[size_t(q0)]) * t : a[0];
    }

    void eval(int u, float v, float* add) const {
        if (v > maxD) return;
        // Curve: the flare that starts at u bends sideways as it goes in.
        if (L.curve != 0.f)
            u = std::min(std::max(int(std::lround(u + L.curve * v * v / (0.5f * frameH))), 0), n - 1);
        const float fade = 1.f - smoothstep(0.6f * maxD, maxD, v);
        float rgb[3] = {0, 0, 0};
        const float glowK = std::exp(-v / glowFall) * L.edgeGlow;
        for (int c = 0; c < 3; ++c) rgb[c] += glow[size_t(u) * 3 + c] * glowK;

        // A hot, tight core at the edge plus a long subtle tail.
        const float core = std::min(std::max(L.core, 0.f), 1.f);
        // (A pinch's hot core is at its focus.)
        const float vc = L.shape == kPinch ? std::fabs(v - pinchPx) : v;
        // Falloff 0: exponential; 1: holds, then drops away (exp of (v/depth)^4).
        const float vd = v / depthPx;
        const float body = L.falloff > 0.f ? std::exp(-std::pow(vd, 1.f + 3.f * std::min(L.falloff, 1.f))) : std::exp(-vd);
        const float fall = ((1.f - core) * body + core * 2.2f * std::exp(-vc / (0.2f * depthPx))) *
                           L.intensity * fade * flicker;
        const float fk = v / maxD * (K - 1);
        const int k0 = std::min(int(fk), K - 2);
        const float t = fk - k0;

        // Streaks radiate from the light: find where the light is (the
        // centroid of what feeds this point) and squeeze the texture toward it
        // as the flare widens, so the streaks fan out like rays.
        float mod = 1.f;
        float streakVal = 0.5f;
        float exitF = exitProf.empty() ? 0.f : exitProf[size_t(u)];
        // Where across the flare this point is (for the rainbow bands).
        float rayCoord = float(u) / std::max(spreadPx, 2.f);
        if (!streak.empty()) {
            const float uu = L.shape == kBlade ? u - v * bladeTan : float(u);
            float xi = uu;
            const float fan = L.shape == kBeam ? 0.f : L.fan;
            if (fan > 0.f && !den.empty()) {
                const float d0 = at1(den[k0], uu, n), d1 = at1(den[k0 + 1], uu, n);
                const float m0 = at1(num[k0], uu, n), m1 = at1(num[k0 + 1], uu, n);
                const float dd = d0 + (d1 - d0) * t, mm = m0 + (m1 - m0) * t;
                if (dd > 1e-6f) {
                    const float cen = mm / dd;
                    const float sp = std::max(spreadPx, 1.f);
                    xi = cen + (uu - cen) * sp / (sp + fan * v);
                    rayCoord = (uu - cen) / (sp + fan * v);
                }
            }
            const float a = at1(streak[k0], xi, n), b = at1(streak[k0 + 1], xi, n);
            streakVal = a + (b - a) * t;
            mod = std::max(0.f, 1.f + 2.f * L.streaks * (streakVal - 0.5f));
        }

        if (L.shape == kPinch) {
            evalPinch(u, v, fall, rgb, rayCoord, streakVal, exitF);
        } else if (L.shape == kPlume || L.shape == kBeam) {
            const float* a = &levels[k0][size_t(u) * 3];
            const float* b = &levels[k0 + 1][size_t(u) * 3];
            for (int c = 0; c < 3; ++c) rgb[c] += (a[c] + (b[c] - a[c]) * t) * fall * mod;
        } else {
            // Blade: a box along the edge, widening with Fan and leaning with
            // Angle: uniform inside, crisp (Blade Softness) at the sides.
            const float hw = bladeHw0 + L.fan * 0.5f * v;
            const float centre = u - v * bladeTan;
            auto cumAt = [&](float q, int c) {
                q = std::min(std::max(q, 0.f), float(n));
                const int q0 = std::min(int(q), n - 1);
                const float tq = q - q0;
                const float a0 = cum[size_t(q0) * 3 + c], a1 = cum[size_t(q0 + 1) * 3 + c];
                return a0 + (a1 - a0) * tq;
            };
            rayCoord = (u - v * bladeTan) / hw;
            const float amp = fall * mod / (2.f * bladeHw0) * std::sqrt(bladeHw0 / hw);
            for (int c = 0; c < 3; ++c) {
                const float hwc = hw * chanScale[c];  // fringe: red's box a little wider
                rgb[c] += (cumAt(centre + hwc + 0.5f, c) - cumAt(centre - hwc + 0.5f, c)) * amp / chanScale[c];
            }
        }
        // The lights' colour, or white.
        float col[3];
        const float lum = 0.2126f * rgb[0] + 0.7152f * rgb[1] + 0.0722f * rgb[2];
        for (int c = 0; c < 3; ++c) col[c] = lum + (rgb[c] - lum) * L.sourceColour;
        // Iridescence: rainbow bands (same brightness, shifted hue), across
        // the depth like thin-film arcs, or along the rays like prism edges,
        // broken up by the streaks.
        if (L.iridescence > 0.f) {
            const float al = std::min(std::max(L.rainbowAlong, 0.f), 1.f);
            const float pe = std::min(std::max(L.prismEdges, 0.f), 1.f);
            const float ph = L.rainbowBands * (al * (0.5f * rayCoord + (0.15f + 1.2f * pe) * streakVal) +
                                               (1.f - al) * (v / std::max(depthPx, 1.f) + 0.35f * streakVal));
            float sp[3];
            for (int c = 0; c < 3; ++c) sp[c] = 0.5f + 0.5f * std::cos(6.2831853f * (ph + c / 3.f));
            const float ls = std::max(0.2126f * sp[0] + 0.7152f * sp[1] + 0.0722f * sp[2], 0.15f);
            // Along the rays, the colour sits on the rays' edges (a narrow band
            // of the streak texture), like a prism's thin spectral lines.
            const float edge = 1.f - smoothstep(0.05f, 0.16f, std::fabs(streakVal - 0.55f));
            // Running along the flare, the hot middle stays white; the colours sit outside it.
            const float mid = al * (1.f - pe) * 0.8f * (1.f - smoothstep(0.1f, 0.55f, std::fabs(rayCoord)));
            const float ir = std::min(std::max(L.iridescence, 0.f), 1.f) * (1.f - pe + pe * edge) * (1.f - mid);
            for (int c = 0; c < 3; ++c) col[c] += (lum * sp[c] / ls - col[c]) * ir;
        }
        // Veil: a wide, low haze (veiling glare), without the streaks.
        if (!veil.empty()) {
            const float vk = L.veil * std::exp(-v / veilFall) * fade * flicker * L.intensity;
            const float* vv = &veil[size_t(u) * 3];
            const float vl = 0.2126f * vv[0] + 0.7152f * vv[1] + 0.0722f * vv[2];
            for (int c = 0; c < 3; ++c) col[c] += (vl + (vv[c] - vl) * L.sourceColour) * vk;
        }
        // Tint; Far Tint: the colour drifts as the flare goes into the frame; Exit
        // Tint: and as the light goes further out of frame (the flare discolours
        // as it dies).
        const float m = 1.f - std::exp(-v / farFall);
        const float xf = smoothstep(0.f, 1.f, exitF);
        for (int c = 0; c < 3; ++c)
            add[c] += weight * col[c] * L.tint[c] * (1.f + (L.farTint[c] - 1.f) * m) * (1.f + (L.exitTint[c] - 1.f) * xf);
    }

    // Pinch: a real focus. Each light's rays come in from the edge, converge
    // to a focal point Pinch % into the frame, cross there and diverge again
    // (Fan sets the angle, the same either side; Spread how tight the focus
    // is). The width follows a smooth hourglass, so the focus is soft. The
    // light's energy is conserved, so the focus is hot and the cone dims as
    // it opens. The rays break up along their length, carry fine filaments
    // and have ragged edges, and blur together at the focus, where they all
    // cross.
    void evalPinch(int u, float v, float fall, float* rgb, float& rayCoord, float& streakVal, float& exitF) const {
        if (srcs.empty()) return;
        float wsum = 0.f, rsum = 0.f, ssum = 0.f, xsum = 0.f;
        const float dv = v - pinchPx;
        const float slope = std::max(0.5f * L.fan, 0.01f);
        const float z0 = neckHw / slope;  // how far either side of the focus it stays tight
        const float hw = std::sqrt(neckHw * neckHw + slope * slope * dv * dv);
        const float cross = dv / std::sqrt(dv * dv + z0 * z0);  // -1 before the focus, +1 after
        const float sh = std::min(std::max(L.shimmer, 0.f), 1.f);
        const float tSh = time * L.shimmerSpeed * 0.3f * sh;
        const float lobes = 3.f + 30.f * std::min(std::max(L.streakDetail, 0.f), 1.f);
        const float alongScale = std::max(1.f, 0.15f * depthPx);
        for (const Src& sr : srcs) {
            const float c0 = sr.cen - leanTan * dv;
            const float rho = (float(u) - c0) / hw;
            if (std::fabs(rho) > 2.2f) continue;
            const float rr = rho * cross;  // which ray (rays swap sides through the focus)
            float tv = 0.5f;
            if (!rays.empty()) {
                const float q = std::min(std::max((rr + 2.f) * 0.25f * (kRays - 1), 0.f), float(kRays - 1));
                // Big rays, broken up along their length, plus fine filaments.
                const float big = at1(rays, q, kRays);
                const float brk = valueNoise2(rr * lobes * 0.5f + 50.f, v / alongScale - tSh, seed + 77u);
                const float fine = valueNoise2(rr * lobes * 4.f + 9.f, v / (2.5f * alongScale) - tSh, seed + 78u);
                tv = 0.6f * big + 0.4f * brk + 0.25f * (fine - 0.5f);
            }
            const float mod = std::max(0.f, 1.f + 2.f * L.streaks * std::fabs(cross) * (tv - 0.5f));
            // Ragged edges.
            const float rag = 1.f + 0.35f * (valueNoise2(v / (0.6f * alongScale), rho > 0.f ? 3.f : 11.f,
                                                         seed + 79u) - 0.5f);
            float wl = 0.f;
            for (int c = 0; c < 3; ++c) {
                const float rc = std::fabs(rho) / (chanScale[c] * rag);
                const float k = std::exp(-2.5f * rc * rc) / (1.12f * hw * chanScale[c]);
                const float add = sr.e[c] * k * fall * mod;
                rgb[c] += add;
                wl += add;
            }
            wsum += wl;
            rsum += wl * rr;
            ssum += wl * tv;
            xsum += wl * sr.exit;
        }
        if (wsum > 0.f) {
            rayCoord = rsum / wsum;
            streakVal = ssum / wsum;
            exitF = xsum / wsum;
        }
    }
};

Flare buildFlare(const Look& L, float weight, const FlareGeom& g, const std::vector<float>& S, float time,
                 unsigned seed) {
    Flare fl;
    fl.L = L;
    fl.weight = weight;
    fl.n = g.n;
    const int n = g.n;
    const float unit = g.unit;
    const float reachPx = std::max(1.f, L.reach * unit);
    const float startPx = L.flareStart * unit;          // + inside the frame, - beyond the edge
    const float rampPx = std::max(1.f, 0.5f * unit);     // comes up quickly once it starts
    fl.depthPx = std::max(1.f, L.depth * unit);
    fl.spreadPx = std::max(0.f, L.spread * unit);
    fl.maxD = fl.depthPx * 5.f;  // exp(-5) < 1%, then faded to zero: never a visible end line
    fl.bladeTan = std::tan(std::min(std::max(L.bladeAngle, -75.f), 75.f) * 3.14159265f / 180.f);
    fl.bladeHw0 = std::max(1.f, fl.spreadPx * 0.5f);
    fl.glowFall = 0.004f * g.h + 1.f;
    const float fr = std::min(std::max(L.fringe, 0.f), 2.f);
    fl.chanScale[0] = 1.f + 0.5f * fr;
    fl.chanScale[2] = std::max(0.2f, 1.f - 0.3f * fr);
    const float sh = std::min(std::max(L.shimmer, 0.f), 1.f);
    fl.flicker = 1.f + 0.25f * sh * (2.f * valueNoise2(time * L.shimmerSpeed * 0.5f, float(g.e), seed + 99u) - 1.f);

    // The light along this edge: from Flare Start (ramping up) out to Reach beyond.
    std::vector<float> prof(size_t(n) * 3, 0.f), exitRaw(size_t(n), 0.f);
    const int v0 = std::min(-int(std::ceil(reachPx * 1.5f)), int(std::floor(startPx - rampPx)) - 1);
    const int v1 = std::max(0, int(std::ceil(startPx)) + 1);
    const bool horiz = g.e == kTop || g.e == kBottom;
    parallelFor(n, [&](int u) {
        float acc[3] = {0, 0, 0}, accL = 0.f, accX = 0.f;
        for (int v = v0; v <= v1; ++v) {
            const int q = int(std::floor(g.edgePos + float(v * g.inward)));
            if (q < 0 || q >= g.span) continue;
            const float fv = float(v);
            const float onset = 1.f - smoothstep(startPx - rampPx, startPx, fv);
            const float beyond = std::min(fv, 0.f) - std::min(startPx - rampPx, 0.f);
            const float wgt = onset * std::exp(std::min(beyond, 0.f) / (0.5f * reachPx));
            const size_t i = horiz ? (size_t(q) * g.w + u) * 3 : (size_t(u) * g.w + q) * 3;
            for (int c = 0; c < 3; ++c) acc[c] += S[i + c] * wgt;
            const float ls = (0.2126f * S[i] + 0.7152f * S[i + 1] + 0.0722f * S[i + 2]) * wgt;
            accL += ls;
            accX += ls * std::max(startPx - fv, 0.f);  // how far past where it starts to flare
        }
        for (int c = 0; c < 3; ++c) prof[size_t(u) * 3 + c] = acc[c] / (0.5f * reachPx);
        exitRaw[size_t(u)] = accL > 0.f ? std::min(accX / accL / (0.7f * reachPx), 1.f) : 0.f;
    });

    const float fan = L.shape == kBeam ? 0.f : L.fan;  // a beam doesn't widen
    std::vector<float> lum(n), lumU(n);
    for (int u = 0; u < n; ++u) {
        const float* p = &prof[size_t(u) * 3];
        lum[u] = 0.2126f * p[0] + 0.7152f * p[1] + 0.0722f * p[2];
        lumU[u] = lum[u] * float(u);
    }
    if (L.exitTint[0] != 1.f || L.exitTint[1] != 1.f || L.exitTint[2] != 1.f) {
        // Where along the edge: the exit amount, averaged over what feeds each point.
        std::vector<float> lx(n);
        for (int u = 0; u < n; ++u) lx[u] = lum[u] * exitRaw[size_t(u)];
        const float r = std::max(2.f, fl.spreadPx + 0.5f * fan * fl.depthPx);
        const std::vector<float> a1 = blur1(lx, n, r), a0 = blur1(lum, n, r);
        fl.exitProf.resize(n);
        for (int u = 0; u < n; ++u) fl.exitProf[size_t(u)] = a0[u] > 1e-9f ? a1[u] / a0[u] : 0.f;
    }
    for (int k = 0; k < K; ++k) {
        const float d = fl.maxD * k / (K - 1);
        const float r0 = fl.spreadPx + fan * d;
        if (L.shape == kPlume || L.shape == kBeam) {
            const float r[3] = {r0 * fl.chanScale[0], r0 * fl.chanScale[1], r0 * fl.chanScale[2]};
            fl.levels.push_back(blurChannels(prof, n, r));
        }
        if (L.streaks > 0.f && fan > 0.f && L.shape != kPinch) {
            fl.den.push_back(blur1(lum, n, std::max(r0, 2.f)));
            fl.num.push_back(blur1(lumU, n, std::max(r0, 2.f)));
        }
    }
    fl.farFall = std::max(1.f, 0.6f * fl.depthPx);
    fl.frameH = float(g.h);
    fl.time = time;
    fl.seed = seed;
    if (L.veil > 0.f) {
        // Veiling glare: the light spread very wide and low, reaching further in.
        fl.veil = blurProfile(prof, n, 0.6f * fl.depthPx + 2.f * fl.spreadPx + 0.05f * g.h);
        fl.veilFall = 1.5f * fl.depthPx;
    }
    if (L.shape == kPinch) {
        fl.pinchPx = std::max(0.f, L.pinch * unit);
        fl.leanTan = std::tan(std::min(std::max(L.lean, -60.f), 60.f) * 3.14159265f / 180.f);
        fl.neckHw = std::max(1.f, 0.5f * fl.spreadPx);
        // Lights along the edge: runs of the profile, merged when closer than the neck.
        float mx = 0.f;
        for (int u = 0; u < n; ++u) mx = std::max(mx, lum[u]);
        const float thr = std::max(mx * 1e-3f, 1e-7f);
        const int gap = std::max(1, int(fl.neckHw));
        for (int u = 0; u < n;) {
            if (lum[u] <= thr) { ++u; continue; }
            Flare::Src sr{0.f, {0.f, 0.f, 0.f}, 0.f};
            float sl = 0.f, slu = 0.f, slx = 0.f;
            int last = u;
            for (; u < n && u - last <= gap; ++u) {
                if (lum[u] <= thr) continue;
                last = u;
                for (int c = 0; c < 3; ++c) sr.e[c] += prof[size_t(u) * 3 + c];
                sl += lum[u];
                slu += lum[u] * float(u);
                slx += lum[u] * exitRaw[size_t(u)];
            }
            if (sl <= 0.f) continue;
            sr.cen = slu / sl;
            sr.exit = slx / sl;
            fl.srcs.push_back(sr);
        }
        std::sort(fl.srcs.begin(), fl.srcs.end(), [](const Flare::Src& a, const Flare::Src& b) {
            return a.e[0] + a.e[1] + a.e[2] > b.e[0] + b.e[1] + b.e[2];
        });
        if (fl.srcs.size() > 16) fl.srcs.resize(16);
        if (L.streaks > 0.f) {
            // A few big rays (Streak Detail 0) to many fine ones (1), shimmering.
            const float lobes = 3.f + 30.f * std::min(std::max(L.streakDetail, 0.f), 1.f);
            const float tSh = time * L.shimmerSpeed * 0.15f;
            fl.rays.resize(Flare::kRays);
            for (int i = 0; i < Flare::kRays; ++i) {
                const float rho = -2.f + 4.f * i / (Flare::kRays - 1);
                float acc = 0.f, norm = 0.f;
                for (int o = 0; o < 3; ++o) {
                    const float fq = float(1 << o), a = 1.f / fq;
                    const float x = (rho * 0.5f * lobes + 100.f * g.e) * fq;
                    const float st = valueNoise2(x, 0.f, seed + 17u * o);
                    const float mv = valueNoise2(x * 1.31f + 57.f, tSh * fq, seed + 101u + 17u * o);
                    acc += a * (st + (mv - st) * sh);
                    norm += a;
                }
                fl.rays[size_t(i)] = acc / norm;
            }
        }
        fl.glow = blurProfile(prof, n, std::max(1.f, fl.spreadPx * 0.4f));
        return fl;
    }
    if (L.shape == kBlade) {
        const std::vector<float> soft = blurProfile(prof, n, std::max(0.f, L.bladeSoftness * unit));
        fl.cum.assign(size_t(n + 1) * 3, 0.f);
        for (int u = 0; u < n; ++u)
            for (int c = 0; c < 3; ++c) fl.cum[size_t(u + 1) * 3 + c] = fl.cum[size_t(u) * 3 + c] + soft[size_t(u) * 3 + c];
    }
    if (L.streaks > 0.f) {
        // 3 octaves of value noise along the edge (the gate's nicks and
        // roughness), blended toward a moving pattern by Shimmer.
        const float spacing = std::max(1.f, g.h * 0.05f * std::pow(0.1f, std::min(std::max(L.streakDetail, 0.f), 1.f)));
        const float tSh = time * L.shimmerSpeed * 0.15f;
        std::vector<float> tex(n);
        for (int u = 0; u < n; ++u) {
            float acc = 0.f, norm = 0.f;
            for (int o = 0; o < 3; ++o) {
                const float fq = float(1 << o), a = 1.f / fq;
                const float x = (u + 1000.f * g.e) / spacing * fq;
                const float st = valueNoise2(x, 0.f, seed + 17u * o);
                const float mv = valueNoise2(x * 1.31f + 57.f, tSh * fq, seed + 101u + 17u * o);
                acc += a * (st + (mv - st) * sh);
                norm += a;
            }
            tex[u] = acc / norm;
        }
        for (int k = 0; k < K; ++k) {
            const float d = fl.maxD * k / (K - 1);
            // Radiating streaks spread by themselves; soften them just a little.
            fl.streak.push_back(blur1(tex, n, (fan > 0.f ? 0.12f : 0.3f) * std::max(fan, 0.3f) * d));
        }
    }
    fl.glow = blurProfile(prof, n, std::max(1.f, fl.spreadPx * 0.4f));
    return fl;
}


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

    // One flare per edge per Look (A and/or B of that edge's pair).
    std::vector<Flare> flares[4];
    for (int e = 0; e < 4; ++e) {
        if (!s.edges[e]) continue;
        const EdgePair& pr = s.pair[e < 2 ? 0 : 1];
        const float b = std::min(std::max(pr.blend, 0.f), 1.f);
        const bool horiz = e == kTop || e == kBottom;
        FlareGeom g;
        g.e = e;
        g.n = horiz ? w : h;
        g.span = horiz ? h : w;
        g.edgePos = e == kTop ? gy0 : e == kBottom ? gy1 : e == kLeft ? gx0 : gx1;
        g.inward = (e == kTop || e == kLeft) ? 1 : -1;
        g.w = w;
        g.unit = unit;
        g.h = h;
        if (b < 1.f) flares[e].push_back(buildFlare(pr.a, 1.f - b, g, S, s.time, s.seed));
        if (b > 0.f) flares[e].push_back(buildFlare(pr.b, b, g, S, s.time, s.seed + 7919u));
    }

    // Composite in linear light. Film: scene-referred footage tops out around
    // 6 stops over mid grey, like FilmFlash's negative.
    const float filmW = 0.18f * 64.f;
    out = Image(w, h);
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
                    if (flares[e].empty()) continue;
                    const float v = e == kTop ? yv - gy0 : e == kBottom ? gy1 - 1 - yv : e == kLeft ? x - gx0 : gx1 - 1 - x;
                    const int u = (e == kTop || e == kBottom) ? x : yv;
                    for (const Flare& fl : flares[e]) fl.eval(u, v, add);
                }
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

const char* lookName(int preset) {
    static const char* const names[kLookCount] = {"Custom", "Soft Haze", "Thin Streak", "Cool Column",
                                                  "Blue Blade", "Side Blast"};
    return preset >= 0 && preset < kLookCount ? names[preset] : "Custom";
}

Look presetLook(int preset, const Look& custom) {
    Look L;  // defaults are Soft Haze
    auto set = [](Look& l, int shape, float in, float start, float reach, float depth, float spread, float fan,
                  float glow, float streaks, float detail, float shimmer, float core, float fringe,
                  float r, float g, float b, float srcCol) {
        l.shape = shape; l.intensity = in; l.flareStart = start; l.reach = reach; l.depth = depth;
        l.spread = spread; l.fan = fan; l.edgeGlow = glow; l.streaks = streaks; l.streakDetail = detail;
        l.shimmer = shimmer; l.core = core; l.fringe = fringe;
        l.tint[0] = r; l.tint[1] = g; l.tint[2] = b; l.sourceColour = srcCol;
    };
    switch (preset) {
        case kSoftHaze:  // a soft, streaky plume in the lights' own colour
            set(L, kPlume, 1.f, 0.f, 6.f, 14.f, 2.5f, 0.6f, 0.35f, 0.5f, 0.5f, 0.3f, 0.4f, 0.3f, 1, 1, 1, 1);
            break;
        case kThinStreak:  // Road to Perdition: a long, thin streak straight in from the side
            set(L, kBeam, 0.45f, 0.f, 8.f, 30.f, 0.6f, 0.f, 0.15f, 0.5f, 0.7f, 0.3f, 0.35f, 0.2f, 1.f, 0.95f, 0.88f, 0.6f);
            break;
        case kCoolColumn:  // a cool column dropping straight down from the top
            set(L, kBeam, 1.f, 0.f, 8.f, 25.f, 3.f, 0.05f, 0.25f, 0.45f, 0.45f, 0.3f, 0.25f, 0.25f, 0.6f, 0.85f, 1.f, 0.f);
            break;
        case kBlueBlade:  // a hard-edged blue wedge, leaning a little
            set(L, kBlade, 1.f, -0.5f, 9.f, 20.f, 2.5f, 0.3f, 0.2f, 0.5f, 0.6f, 0.35f, 0.5f, 0.4f, 0.45f, 0.7f, 1.f, 0.f);
            L.bladeAngle = 8.f;
            L.bladeSoftness = 0.4f;
            break;
        case kSideBlast:  // Gummo: the light comes to a focus just inside the edge, crosses and opens
                          // into a soft, broken-up blue cone that turns green-teal as the light goes
                          // further out, just after the light has gone
            set(L, kPinch, 1.f, -0.8f, 10.f, 20.f, 1.2f, 0.35f, 0.05f, 1.f, 0.3f, 0.5f, 0.35f, 0.3f, 0.8f, 0.9f, 1.f, 0.f);
            L.pinch = 12.f;
            L.lean = 0.f;
            L.farTint[0] = 0.35f; L.farTint[1] = 0.6f; L.farTint[2] = 1.f;
            L.exitTint[0] = 0.8f; L.exitTint[1] = 1.15f; L.exitTint[2] = 0.45f;
            L.veil = 0.3f;
            L.shimmerSpeed = 1.5f;
            break;
        default:
            return custom;
    }
    return L;
}

}  // namespace gate
