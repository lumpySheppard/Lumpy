// Lumpy.app: a native window around Lumpy's interface. It starts the
// bundled `lumpy ui --app` engine, shows its page in a WKWebView, and
// gives the page native folder pickers, Save dialogs and Show in Finder.
import Cocoa
import UniformTypeIdentifiers
import WebKit

/// A mutable value shared with a callback that may run on another thread.
final class Box<T> {
    var value: T
    init(_ value: T) { self.value = value }
}

final class AppDelegate: NSObject, NSApplicationDelegate, WKScriptMessageHandler, WKUIDelegate, WKNavigationDelegate {
    var window: NSWindow!
    var web: WKWebView!
    var engine: Process?
    let engineInput = Pipe()
    var quitting = false

    func applicationDidFinishLaunching(_ notification: Notification) {
        buildMenu()

        let config = WKWebViewConfiguration()
        config.userContentController.add(self, name: "lumpy")
        web = WKWebView(frame: .zero, configuration: config)
        web.uiDelegate = self
        web.navigationDelegate = self
        web.setValue(false, forKey: "drawsBackground") // no white flash while loading

        window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 1320, height: 860),
            styleMask: [.titled, .closable, .miniaturizable, .resizable],
            backing: .buffered, defer: false)
        window.title = "Lumpy"
        window.titlebarAppearsTransparent = true
        window.backgroundColor = NSColor(name: nil) { appearance in
            appearance.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua
                ? NSColor(red: 0.106, green: 0.106, blue: 0.114, alpha: 1)
                : NSColor(red: 0.953, green: 0.953, blue: 0.961, alpha: 1)
        }
        window.minSize = NSSize(width: 760, height: 520)
        window.contentView = web
        window.center()
        window.setFrameAutosaveName("LumpyMainWindow")
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)

        startEngine()
    }

    func startEngine() {
        guard let bin = Bundle.main.url(forResource: "lumpy", withExtension: nil) else {
            fail("The Lumpy engine is missing from the app bundle.")
            return
        }
        let p = Process()
        p.executableURL = bin
        p.arguments = ["ui", "--app"]
        p.standardInput = engineInput
        let out = Pipe()
        p.standardOutput = out
        p.standardError = logHandle()
        let loaded = Box(false)
        out.fileHandleForReading.readabilityHandler = { [weak self] handle in
            let data = handle.availableData // keep draining so the engine never blocks
            guard !loaded.value, let text = String(data: data, encoding: .utf8) else { return }
            for line in text.split(separator: "\n") where line.hasPrefix("LUMPY_URL ") {
                loaded.value = true
                if let url = URL(string: String(line.dropFirst("LUMPY_URL ".count))) {
                    DispatchQueue.main.async { self?.web.load(URLRequest(url: url)) }
                }
            }
        }
        p.terminationHandler = { [weak self] proc in
            DispatchQueue.main.async {
                guard let self = self, !self.quitting else { return }
                self.fail("The Lumpy engine stopped (exit status \(proc.terminationStatus)). See ~/Library/Logs/Lumpy/app.log.")
            }
        }
        do {
            try p.run()
            engine = p
        } catch {
            fail("Could not start the Lumpy engine: \(error.localizedDescription)")
        }
    }

    func logHandle() -> FileHandle {
        let dir = FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent("Library/Logs/Lumpy")
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let file = dir.appendingPathComponent("app.log")
        if !FileManager.default.fileExists(atPath: file.path) {
            FileManager.default.createFile(atPath: file.path, contents: nil)
        }
        guard let h = try? FileHandle(forWritingTo: file) else { return FileHandle.nullDevice }
        h.seekToEndOfFile()
        return h
    }

    func fail(_ message: String) {
        let alert = NSAlert()
        alert.messageText = "Lumpy can't continue"
        alert.informativeText = message
        alert.alertStyle = .critical
        alert.runModal()
        NSApp.terminate(nil)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        if web.url != nil, hasRunningJobs() {
            let alert = NSAlert()
            alert.messageText = "An offload or check is still running"
            alert.informativeText = "Quitting stops it. Files already verified stay verified, and running the same offload again finishes the rest."
            alert.addButton(withTitle: "Keep Running")
            alert.addButton(withTitle: "Quit Anyway")
            if alert.runModal() == .alertFirstButtonReturn { return .terminateCancel }
        }
        return .terminateNow
    }

    // Asks the page synchronously-enough (spinning the run loop briefly)
    // whether jobs are running, from the Activity counter.
    func hasRunningJobs() -> Bool {
        let result = Box<Bool?>(nil)
        web.evaluateJavaScript("(document.getElementById('count-jobs')||{}).textContent || ''") { value, _ in
            result.value = ((value as? String) ?? "") != ""
        }
        let deadline = Date().addingTimeInterval(1)
        while result.value == nil && Date() < deadline {
            RunLoop.current.run(mode: .default, before: Date().addingTimeInterval(0.02))
        }
        return result.value ?? false
    }

    func applicationWillTerminate(_ notification: Notification) {
        quitting = true
        try? engineInput.fileHandleForWriting.close() // the engine exits when its input closes
        if let e = engine, e.isRunning {
            DispatchQueue.global().asyncAfter(deadline: .now() + 2) { if e.isRunning { e.terminate() } }
            e.waitUntilExit()
        }
    }

    // MARK: page -> native calls

    func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage) {
        guard let body = message.body as? [String: Any], let action = body["action"] as? String,
              let id = body["id"] as? Int else { return }
        switch action {
        case "pickFolder":
            let panel = NSOpenPanel()
            panel.canChooseDirectories = true
            panel.canChooseFiles = false
            panel.canCreateDirectories = true
            panel.allowsMultipleSelection = false
            panel.prompt = "Choose"
            if let start = body["start"] as? String, !start.isEmpty {
                panel.directoryURL = URL(fileURLWithPath: start)
            } else {
                panel.directoryURL = URL(fileURLWithPath: "/Volumes")
            }
            panel.beginSheetModal(for: window) { response in
                self.resolve(id, response == .OK ? panel.url?.path : nil)
            }
        case "saveFile":
            let panel = NSSavePanel()
            panel.nameFieldStringValue = body["name"] as? String ?? "lumpy.csv"
            panel.allowedContentTypes = [.commaSeparatedText]
            panel.canCreateDirectories = true
            panel.beginSheetModal(for: window) { response in
                guard response == .OK, let url = panel.url else { return self.resolve(id, nil) }
                do {
                    try (body["content"] as? String ?? "").write(to: url, atomically: true, encoding: .utf8)
                    self.resolve(id, url.lastPathComponent)
                } catch {
                    self.resolve(id, nil)
                }
            }
        case "reveal":
            if let path = body["path"] as? String {
                NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: path)])
            }
            resolve(id, true)
        default:
            resolve(id, nil)
        }
    }

    func resolve(_ id: Int, _ value: Any?) {
        var json = "null"
        if let value = value, let data = try? JSONSerialization.data(withJSONObject: value, options: .fragmentsAllowed),
           let s = String(data: data, encoding: .utf8) {
            json = s
        }
        web.evaluateJavaScript("window.lumpyNative.resolve(\(id), \(json))")
    }

    // MARK: JavaScript alert() and confirm()

    func webView(_ webView: WKWebView, runJavaScriptAlertPanelWithMessage message: String,
                 initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping () -> Void) {
        let alert = NSAlert()
        alert.messageText = message
        alert.beginSheetModal(for: window) { _ in completionHandler() }
    }

    func webView(_ webView: WKWebView, runJavaScriptConfirmPanelWithMessage message: String,
                 initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (Bool) -> Void) {
        let alert = NSAlert()
        alert.messageText = message
        alert.addButton(withTitle: "OK")
        alert.addButton(withTitle: "Cancel")
        alert.beginSheetModal(for: window) { completionHandler($0 == .alertFirstButtonReturn) }
    }

    // Links to other sites open in the default browser.
    func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction,
                 decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        if let url = action.request.url, url.host != "127.0.0.1", url.host != "localhost", url.scheme != "about" {
            NSWorkspace.shared.open(url)
            decisionHandler(.cancel)
            return
        }
        decisionHandler(.allow)
    }

    // MARK: menus

    func buildMenu() {
        let main = NSMenu()
        func item(_ title: String, _ action: Selector?, _ key: String, _ mods: NSEvent.ModifierFlags = .command) -> NSMenuItem {
            let i = NSMenuItem(title: title, action: action, keyEquivalent: key)
            i.keyEquivalentModifierMask = mods
            return i
        }
        let appMenu = NSMenu()
        appMenu.addItem(item("About Lumpy", #selector(NSApplication.orderFrontStandardAboutPanel(_:)), ""))
        appMenu.addItem(.separator())
        appMenu.addItem(item("Hide Lumpy", #selector(NSApplication.hide(_:)), "h"))
        appMenu.addItem(item("Hide Others", #selector(NSApplication.hideOtherApplications(_:)), "h", [.command, .option]))
        appMenu.addItem(item("Show All", #selector(NSApplication.unhideAllApplications(_:)), ""))
        appMenu.addItem(.separator())
        appMenu.addItem(item("Quit Lumpy", #selector(NSApplication.terminate(_:)), "q"))

        let edit = NSMenu(title: "Edit")
        edit.addItem(item("Undo", Selector(("undo:")), "z"))
        edit.addItem(item("Redo", Selector(("redo:")), "z", [.command, .shift]))
        edit.addItem(.separator())
        edit.addItem(item("Cut", #selector(NSText.cut(_:)), "x"))
        edit.addItem(item("Copy", #selector(NSText.copy(_:)), "c"))
        edit.addItem(item("Paste", #selector(NSText.paste(_:)), "v"))
        edit.addItem(item("Select All", #selector(NSText.selectAll(_:)), "a"))

        let view = NSMenu(title: "View")
        view.addItem(item("Reload", #selector(reload), "r"))
        view.addItem(item("Enter Full Screen", #selector(NSWindow.toggleFullScreen(_:)), "f", [.command, .control]))

        let win = NSMenu(title: "Window")
        win.addItem(item("Minimize", #selector(NSWindow.performMiniaturize(_:)), "m"))
        win.addItem(item("Zoom", #selector(NSWindow.performZoom(_:)), ""))
        NSApp.windowsMenu = win

        for (title, menu) in [("Lumpy", appMenu), ("Edit", edit), ("View", view), ("Window", win)] {
            let top = NSMenuItem(title: title, action: nil, keyEquivalent: "")
            top.submenu = menu
            main.addItem(top)
        }
        NSApp.mainMenu = main
    }

    @objc func reload() { web.reload() }
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.setActivationPolicy(.regular)
app.run()
