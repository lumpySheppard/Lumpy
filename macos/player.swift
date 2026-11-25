// The clip player: a real AVPlayer, laid over the web view exactly
// where the page's player sits, driven by the page's own transport.
// The page sends the file and the rectangle; the player sends back
// every tick of its clock, so the timecode is the decoder's, frames
// step exactly, and nothing is decoded twice.

import AVFoundation
import AVKit
import Cocoa
import WebKit

/// The view over the page. Clicks go to the page as play/pause, the
/// scroll wheel goes on to the web view so the page still scrolls.
final class PlayerView: AVPlayerView {
    var onClick: ((Int) -> Void)?
    override func mouseDown(with event: NSEvent) { onClick?(event.clickCount) }
    override func scrollWheel(with event: NSEvent) { superview?.scrollWheel(with: event) }
    override var acceptsFirstResponder: Bool { false }
}

final class NativePlayer {
    let web: WKWebView
    var player: AVPlayer?
    var view: PlayerView?
    var pid = 0
    private var timeObserver: Any?
    private var statusObserver: NSKeyValueObservation?
    private var endObserver: NSObjectProtocol?
    private var lastTick = -1.0
    private var lastRate: Float = 0

    init(web: WKWebView) { self.web = web }

    /// A rectangle in the page's CSS pixels becomes one in the web
    /// view's coordinates (which are flipped on macOS, like the page).
    func rect(_ body: [String: Any]) -> NSRect {
        guard let r = body["rect"] as? [String: Any] else { return .zero }
        let x = (r["x"] as? Double) ?? 0, y = (r["y"] as? Double) ?? 0
        let w = max(0, (r["w"] as? Double) ?? 0), h = max(0, (r["h"] as? Double) ?? 0)
        let top = web.isFlipped ? y : Double(web.bounds.height) - y - h
        return NSRect(x: x, y: top, width: w, height: h)
    }

    func open(_ body: [String: Any]) -> Bool {
        close()
        guard let path = body["path"] as? String else { return false }
        pid = (body["pid"] as? Int) ?? 0
        let item = AVPlayerItem(url: URL(fileURLWithPath: path))
        let p = AVPlayer(playerItem: item)
        p.actionAtItemEnd = .pause
        p.automaticallyWaitsToMinimizeStalling = false
        p.isMuted = (body["muted"] as? Bool) ?? false
        let v = PlayerView(frame: rect(body))
        v.player = p
        v.controlsStyle = .none
        v.videoGravity = .resizeAspect
        v.showsFullScreenToggleButton = false
        v.wantsLayer = true
        v.layer?.backgroundColor = NSColor.black.cgColor
        v.onClick = { [weak self] count in self?.send("click", "\(count)") }
        web.addSubview(v)
        player = p
        view = v
        // A tick every 1/60 s: the page draws the timecode from these.
        timeObserver = p.addPeriodicTimeObserver(forInterval: CMTime(value: 1, timescale: 60), queue: .main) { [weak self] t in
            self?.tick(t)
        }
        statusObserver = item.observe(\.status, options: [.new]) { [weak self] item, _ in
            guard let self = self else { return }
            if item.status == .failed {
                self.send("failed", NativePlayer.jsString(item.error?.localizedDescription ?? "cannot play"))
                return
            }
            guard item.status == .readyToPlay else { return }
            var fps = 0.0, w = 0.0, h = 0.0
            if let track = item.asset.tracks(withMediaType: .video).first {
                fps = Double(track.nominalFrameRate)
                let size = track.naturalSize.applying(track.preferredTransform)
                w = Double(abs(size.width)); h = Double(abs(size.height))
            }
            let dur = CMTimeGetSeconds(item.duration)
            self.send("ready", "\(dur.isFinite ? dur : 0), \(fps), \(w), \(h), \(item.canPlayReverse)")
        }
        endObserver = NotificationCenter.default.addObserver(forName: .AVPlayerItemDidPlayToEndTime, object: item, queue: .main) { [weak self] _ in
            self?.send("ended", "")
        }
        return true
    }

    func close() {
        if let o = timeObserver, let p = player { p.removeTimeObserver(o) }
        timeObserver = nil
        statusObserver = nil
        if let e = endObserver { NotificationCenter.default.removeObserver(e) }
        endObserver = nil
        player?.pause()
        view?.removeFromSuperview()
        view = nil
        player = nil
        lastTick = -1
    }

    /// The page's player of this id is the live one; a stale page timer
    /// closing an older player is ignored.
    func owns(_ body: [String: Any]) -> Bool { ((body["pid"] as? Int) ?? pid) == pid }

    func play(rate: Double) {
        guard let p = player else { return }
        p.rate = Float(rate)
        tick(p.currentTime(), force: true)
    }

    func pause() {
        guard let p = player else { return }
        p.pause()
        tick(p.currentTime(), force: true)
    }

    func seek(to seconds: Double, fast: Bool) {
        guard let p = player else { return }
        let t = CMTime(seconds: max(0, seconds), preferredTimescale: 600)
        let tol = fast ? CMTime(seconds: 0.25, preferredTimescale: 600) : CMTime.zero
        p.seek(to: t, toleranceBefore: tol, toleranceAfter: tol) { [weak self] _ in
            guard let self = self, let p = self.player else { return }
            self.tick(p.currentTime(), force: true)
        }
    }

    func step(by n: Int) {
        guard let p = player, let item = p.currentItem else { return }
        p.pause()
        item.step(byCount: n)
        tick(p.currentTime(), force: true)
    }

    func setMuted(_ on: Bool) { player?.isMuted = on }

    func show(_ on: Bool) { view?.isHidden = !on }

    func move(_ body: [String: Any]) { view?.frame = rect(body) }

    /// The frame on screen now, rendered by AVFoundation at the clip's
    /// own size, written as a PNG into a temp folder for the engine to
    /// file. Done off the main thread; the page gets the file's path.
    func grab(done: @escaping ([String: Any]) -> Void) {
        guard let p = player, let item = p.currentItem else { done(["error": "nothing is playing"]); return }
        let t = p.currentTime()
        let gen = AVAssetImageGenerator(asset: item.asset)
        gen.appliesPreferredTrackTransform = true
        gen.requestedTimeToleranceBefore = .zero
        gen.requestedTimeToleranceAfter = .zero
        DispatchQueue.global(qos: .userInitiated).async {
            do {
                let cg = try gen.copyCGImage(at: t, actualTime: nil)
                let rep = NSBitmapImageRep(cgImage: cg)
                guard let png = rep.representation(using: .png, properties: [:]) else { throw NSError(domain: "Lumpy", code: 1, userInfo: [NSLocalizedDescriptionKey: "could not encode the frame"]) }
                let dir = FileManager.default.temporaryDirectory.appendingPathComponent("lumpy-stills", isDirectory: true)
                try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
                let file = dir.appendingPathComponent("grab-\(Int(Date().timeIntervalSince1970 * 1000)).png")
                try png.write(to: file)
                DispatchQueue.main.async { done(["file": file.path, "width": cg.width, "height": cg.height, "t": CMTimeGetSeconds(t)]) }
            } catch {
                DispatchQueue.main.async { done(["error": error.localizedDescription]) }
            }
        }
    }

    private func tick(_ t: CMTime, force: Bool = false) {
        guard let p = player else { return }
        let s = CMTimeGetSeconds(t)
        guard s.isFinite else { return }
        if !force && s == lastTick && p.rate == lastRate { return }
        lastTick = s
        lastRate = p.rate
        send("tick", "\(s), \(p.rate)")
    }

    /// A string as a JavaScript literal, whatever is in it.
    static func jsString(_ s: String) -> String {
        if let data = try? JSONSerialization.data(withJSONObject: s, options: .fragmentsAllowed), let out = String(data: data, encoding: .utf8) { return out }
        return "\"\"" 
    }

    private func send(_ event: String, _ args: String) {
        let call = args.isEmpty ? "window.lumpyPlayer && window.lumpyPlayer.\(event)(\(pid))" : "window.lumpyPlayer && window.lumpyPlayer.\(event)(\(pid), \(args))"
        web.evaluateJavaScript(call)
    }
}
