// Lumpy for iPad: the Space tab over the folders the Files app grants
// (On My iPad, a plugged-in drive, iCloud Drive, and the folders other
// apps expose there, DaVinci Resolve's among them). The engine runs
// inside the app (Mobile.xcframework, from the Go code in mobile/) and
// the page is the same one the Mac app shows, in Space-only mode.

import SwiftUI
import QuickLookThumbnailing
import UniformTypeIdentifiers
import WebKit
import Mobile

@main
struct LumpyApp: App {
    @StateObject private var engine = Engine()

    var body: some Scene {
        WindowGroup {
            ContentView().environmentObject(engine)
        }
    }
}

/// The engine: started once, with every granted folder handed to it
/// again on each launch (a security-scoped bookmark is how iOS lets an
/// app keep a folder the user chose).
final class Engine: ObservableObject {
    @Published var url: URL?
    @Published var error: String?
    private var scoped: [URL] = []

    init() { start() }

    func start() {
        let docs = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
        var err: NSError?
        let u = MobileStart(docs.path, &err)
        if let err = err { error = err.localizedDescription; return }
        for b in bookmarks() { grant(bookmark: b) }
        url = URL(string: u + "&mode=space")
    }

    private func bookmarks() -> [Data] { UserDefaults.standard.array(forKey: "places") as? [Data] ?? [] }

    /// A folder the user just picked: kept as a bookmark, opened for
    /// reading, and given to the engine as a place.
    func add(folder: URL) -> String? {
        guard folder.startAccessingSecurityScopedResource() else { return nil }
        scoped.append(folder)
        if let b = try? folder.bookmarkData(options: .minimalBookmark, includingResourceValuesForKeys: nil, relativeTo: nil) {
            var all = bookmarks()
            all.append(b)
            UserDefaults.standard.set(all, forKey: "places")
        }
        var err: NSError?
        MobileAddPlace(folder.lastPathComponent, folder.path, &err)
        return err == nil ? folder.path : nil
    }

    private func grant(bookmark: Data) {
        var stale = false
        guard let u = try? URL(resolvingBookmarkData: bookmark, options: [], relativeTo: nil, bookmarkDataIsStale: &stale), u.startAccessingSecurityScopedResource() else { return }
        scoped.append(u)
        var err: NSError?
        MobileAddPlace(u.lastPathComponent, u.path, &err)
    }
}

struct ContentView: View {
    @EnvironmentObject var engine: Engine

    var body: some View {
        Group {
            if let url = engine.url {
                WebView(url: url).ignoresSafeArea(edges: .bottom)
            } else if let e = engine.error {
                VStack(spacing: 12) {
                    Text("Lumpy could not start").font(.headline)
                    Text(e).foregroundStyle(.secondary)
                }.padding()
            } else {
                ProgressView("Starting…")
            }
        }
    }
}

/// The page, with the same message bridge the Mac app offers: the page
/// asks for a folder with pickFolder and gets its path back.
struct WebView: UIViewRepresentable {
    @EnvironmentObject var engine: Engine
    let url: URL

    func makeCoordinator() -> Coordinator { Coordinator(engine: engine) }

    func makeUIView(context: Context) -> WKWebView {
        let config = WKWebViewConfiguration()
        config.userContentController.add(context.coordinator, name: "lumpy")
        config.allowsInlineMediaPlayback = true
        let web = WKWebView(frame: .zero, configuration: config)
        web.isOpaque = false
        web.backgroundColor = .systemBackground
        web.scrollView.bounces = false
        context.coordinator.web = web
        web.load(URLRequest(url: url))
        return web
    }

    func updateUIView(_ uiView: WKWebView, context: Context) {}

    final class Coordinator: NSObject, WKScriptMessageHandler, UIDocumentPickerDelegate {
        let engine: Engine
        weak var web: WKWebView?
        private var pending: Int?

        init(engine: Engine) { self.engine = engine }

        func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage) {
            guard let body = message.body as? [String: Any], let action = body["action"] as? String, let id = body["id"] as? Int else { return }
            switch action {
            case "pickFolder":
                pending = id
                let picker = UIDocumentPickerViewController(forOpeningContentTypes: [.folder])
                picker.delegate = self
                picker.allowsMultipleSelection = false
                top()?.present(picker, animated: true)
            case "reveal":
                // The Files app, at the folder.
                if let p = body["path"] as? String, let u = URL(string: "shareddocuments://" + p) {
                    UIApplication.shared.open(u)
                }
                resolve(id, true)
            case "open":
                if let s = body["url"] as? String, let u = URL(string: s) { UIApplication.shared.open(u) }
                resolve(id, true)
            case "thumb":
                // A picture of the file, the way the Files app would
                // draw it: stills, HEIC, a frame of a clip, a PDF's
                // first page, raws. Comes back as a data URL.
                guard let p = body["path"] as? String else { resolve(id, nil); return }
                let w = CGFloat((body["w"] as? Int) ?? 320)
                let req = QLThumbnailGenerator.Request(fileAt: URL(fileURLWithPath: p), size: CGSize(width: w, height: w), scale: 1, representationTypes: .thumbnail)
                QLThumbnailGenerator.shared.generateBestRepresentation(for: req) { rep, _ in
                    let data = rep?.uiImage.jpegData(compressionQuality: 0.82)
                    DispatchQueue.main.async {
                        self.resolve(id, data.map { "data:image/jpeg;base64," + $0.base64EncodedString() })
                    }
                }
            default:
                resolve(id, nil)
            }
        }

        func documentPicker(_ controller: UIDocumentPickerViewController, didPickDocumentsAt urls: [URL]) {
            guard let id = pending else { return }
            pending = nil
            resolve(id, urls.first.flatMap { engine.add(folder: $0) })
        }

        func documentPickerWasCancelled(_ controller: UIDocumentPickerViewController) {
            if let id = pending { pending = nil; resolve(id, nil) }
        }

        private func resolve(_ id: Int, _ value: Any?) {
            var json = "null"
            if let v = value, let data = try? JSONSerialization.data(withJSONObject: v, options: .fragmentsAllowed), let s = String(data: data, encoding: .utf8) { json = s }
            web?.evaluateJavaScript("window.lumpyNative.resolve(\(id), \(json))")
        }

        private func top() -> UIViewController? {
            let scenes = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
            var vc = scenes.flatMap { $0.windows }.first { $0.isKeyWindow }?.rootViewController
            while let p = vc?.presentedViewController { vc = p }
            return vc
        }
    }
}
