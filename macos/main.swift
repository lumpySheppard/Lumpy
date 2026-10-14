// Lumpy.app: a native window around Lumpy's interface. It starts the
// bundled `lumpy ui --app` engine, shows its page in a WKWebView, and
// gives the page native folder pickers, Save dialogs and Show in Finder.
import Cocoa
import UniformTypeIdentifiers
import UserNotifications
import WebKit

/// A mutable value shared with a callback that may run on another thread.
final class Box<T> {
    var value: T
    init(_ value: T) { self.value = value }
}

final class AppDelegate: NSObject, NSApplicationDelegate, WKScriptMessageHandler, WKUIDelegate, WKNavigationDelegate, UNUserNotificationCenterDelegate {
    var window: NSWindow!
    var web: WKWebView!
    var engine: Process?
    let engineInput = Pipe()
    var quitting = false
    var engineURL: URL?

    // The menu bar item and its panel: running transfers, plugged-in
    // drives and a few numbers, from any app.
    var statusItem: NSStatusItem!
    let popover = NSPopover()
    var menuWeb: WKWebView!
    var statusTimer: Timer?
    // The last state seen for each job, so a job that ends between two
    // polls gets one notification; nothing is posted for what was
    // already over when polling began.
    var jobStates: [Int: String] = [:]
    var jobsPrimed = false

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
        window.isReleasedWhenClosed = false // closing hides; the menu bar item brings it back
        window.contentView = web
        window.center()
        window.setFrameAutosaveName("LumpyMainWindow")
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)

        setupStatusItem()
        startEngine()
        setupNotifications()
    }

    // MARK: notifications

    /// Job notifications come from the engine's view of each job: when a
    /// job it marked for notifying ends, the app posts one.
    func setupNotifications() {
        let center = UNUserNotificationCenter.current()
        center.delegate = self
        center.requestAuthorization(options: [.alert, .sound]) { _, _ in }
    }

    func postNotification(title: String, body: String, id: String) {
        let content = UNMutableNotificationContent()
        content.title = title
        content.body = body
        content.sound = .default
        UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: id, content: content, trigger: nil))
    }

    /// Shown even while Lumpy is the front app: the job may have run
    /// for an hour with the window behind something else.
    func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification,
                                withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void) {
        completionHandler([.banner, .sound])
    }

    /// Clicking a notification brings the window up.
    func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse,
                                withCompletionHandler completionHandler: @escaping () -> Void) {
        DispatchQueue.main.async { self.showWindow() }
        completionHandler()
    }

    // MARK: menu bar

    func setupStatusItem() {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        if let button = statusItem.button {
            button.image = menuBarIcon()
            button.imagePosition = .imageLeading
            button.target = self
            button.action = #selector(togglePopover(_:))
            button.toolTip = "Lumpy"
        }
        let config = WKWebViewConfiguration()
        config.userContentController.add(self, name: "lumpy")
        menuWeb = WKWebView(frame: NSRect(x: 0, y: 0, width: 320, height: 260), configuration: config)
        menuWeb.setValue(false, forKey: "drawsBackground") // the material shows through the page
        menuWeb.navigationDelegate = self
        menuWeb.autoresizingMask = [.width, .height]
        // A translucent dark material, like the system's own menu bar panels.
        let material = NSVisualEffectView(frame: menuWeb.frame)
        material.material = .hudWindow
        material.blendingMode = .behindWindow
        material.state = .active
        material.autoresizingMask = [.width, .height]
        material.addSubview(menuWeb)
        let vc = NSViewController()
        vc.view = material
        popover.contentViewController = vc
        popover.contentSize = NSSize(width: 320, height: 260)
        popover.behavior = .transient
        popover.animates = true
        popover.appearance = NSAppearance(named: .vibrantDark)
        statusTimer = Timer.scheduledTimer(withTimeInterval: 3, repeats: true) { [weak self] _ in self?.pollStatus() }
    }

    /// The NAS-with-an-L glyph as a template image, so it follows the
    /// menu bar's light or dark style.
    func menuBarIcon() -> NSImage {
        let image = NSImage(size: NSSize(width: 18, height: 16), flipped: false) { _ in
            NSColor.black.setStroke()
            NSColor.black.setFill()
            let box = NSBezierPath(roundedRect: NSRect(x: 0.75, y: 0.75, width: 16.5, height: 14.5), xRadius: 3.5, yRadius: 3.5)
            box.lineWidth = 1.5
            box.stroke()
            for i in 0..<3 {
                NSBezierPath(roundedRect: NSRect(x: 3.5, y: CGFloat(3 + i * 4), width: 5, height: 1.8), xRadius: 0.9, yRadius: 0.9).fill()
            }
            let l = NSBezierPath()
            l.move(to: NSPoint(x: 11, y: 11.9)) // as tall as the three bays
            l.line(to: NSPoint(x: 11, y: 3.9))
            l.line(to: NSPoint(x: 14.5, y: 3.9))
            l.lineWidth = 1.8
            l.lineCapStyle = .round
            l.lineJoinStyle = .round
            l.stroke()
            return true
        }
        image.isTemplate = true
        return image
    }

    @objc func togglePopover(_ sender: Any?) {
        if popover.isShown {
            popover.performClose(sender)
            return
        }
        guard let button = statusItem.button, let base = engineURL else { return }
        let page = base.absoluteString.replacingOccurrences(of: "/#t=", with: "/menubar.html#t=")
        if let url = URL(string: page) {
            menuWeb.load(URLRequest(url: url))
        }
        popover.show(relativeTo: button.bounds, of: button, preferredEdge: .minY)
        popover.contentViewController?.view.window?.makeKey()
    }

    /// While a job runs, its progress shows next to the menu bar icon.
    func pollStatus() {
        guard let base = engineURL, var comps = URLComponents(url: base, resolvingAgainstBaseURL: false) else { return }
        let token = (base.fragment ?? "").replacingOccurrences(of: "t=", with: "")
        comps.fragment = nil
        comps.path = "/api/jobs"
        guard let url = comps.url else { return }
        var req = URLRequest(url: url)
        req.setValue(token, forHTTPHeaderField: "X-Lumpy-Token")
        URLSession.shared.dataTask(with: req) { [weak self] data, _, _ in
            guard let data = data, let jobs = try? JSONSerialization.jsonObject(with: data) as? [[String: Any]] else { return }
            DispatchQueue.main.async { self?.noteJobs(jobs) }
            var title = ""
            for job in jobs where (job["state"] as? String) == "running" {
                if let p = job["progress"] as? [String: Any], let done = p["done"] as? Double, let total = p["total"] as? Double, total > 0 {
                    title = " \(Int(done / total * 100))%"
                } else {
                    title = " …"
                }
                break
            }
            DispatchQueue.main.async { self?.statusItem.button?.title = title }
        }.resume()
    }

    /// Posts a notification for each job that has ended since the last
    /// poll and carries a notify text.
    func noteJobs(_ jobs: [[String: Any]]) {
        for job in jobs {
            guard let id = job["id"] as? Int, let state = job["state"] as? String else { continue }
            let was = jobStates[id]
            jobStates[id] = state
            let live = state == "running" || state == "queued"
            guard jobsPrimed, !live, let prev = was, prev == "running" || prev == "queued" else { continue }
            if let text = job["notify"] as? String, !text.isEmpty {
                postNotification(title: job["title"] as? String ?? "Lumpy", body: text, id: "job-\(id)")
            }
        }
        jobsPrimed = true
    }

    func showWindow() {
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows: Bool) -> Bool {
        showWindow()
        return true
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
                    DispatchQueue.main.async {
                        self?.engineURL = url
                        self?.web.load(URLRequest(url: url))
                    }
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

    // Closing the window leaves Lumpy in the menu bar: logging drives,
    // publishing the status page, finishing jobs. Quit is in the menu.
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }

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
        let from = message.webView
        switch action {
        case "openApp":
            popover.performClose(nil)
            showWindow()
            if let again = body["again"] as? Int {
                web.evaluateJavaScript("window.lumpyRunAgain && window.lumpyRunAgain(\(again))")
            }
            resolve(id, true, in: from)
        case "quit":
            resolve(id, true, in: from)
            popover.performClose(nil)
            NSApp.terminate(nil)
        case "appearance":
            // The window chrome follows the page's light or dark theme.
            let light = (body["theme"] as? String) == "light"
            window.appearance = NSAppearance(named: light ? .aqua : .darkAqua)
            resolve(id, true, in: from)
        case "resize":
            if let h = body["height"] as? Double {
                popover.contentSize = NSSize(width: 320, height: min(max(h + 2, 100), 640))
            }
            resolve(id, true, in: from)
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
                self.resolve(id, response == .OK ? panel.url?.path : nil, in: from)
            }
        case "pickFiles":
            let panel = NSOpenPanel()
            panel.canChooseDirectories = false
            panel.canChooseFiles = true
            panel.allowsMultipleSelection = true
            panel.prompt = "Choose"
            panel.message = body["message"] as? String ?? "Choose one or more timeline exports: FCPXML, XML, EDL, or a list of clip names"
            panel.beginSheetModal(for: window) { response in
                self.resolve(id, response == .OK ? panel.urls.map { $0.path } : nil, in: from)
            }
        case "saveFile":
            let panel = NSSavePanel()
            panel.nameFieldStringValue = body["name"] as? String ?? "lumpy.csv"
            panel.allowedContentTypes = [.commaSeparatedText]
            panel.canCreateDirectories = true
            panel.beginSheetModal(for: window) { response in
                guard response == .OK, let url = panel.url else { return self.resolve(id, nil, in: from) }
                do {
                    try (body["content"] as? String ?? "").write(to: url, atomically: true, encoding: .utf8)
                    self.resolve(id, url.lastPathComponent, in: from)
                } catch {
                    self.resolve(id, nil, in: from)
                }
            }
        case "reveal":
            if let path = body["path"] as? String {
                NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: path)])
            }
            resolve(id, true, in: from)
        case "relaunch":
            // After an update: quit, and have a detached shell reopen the
            // (now replaced) bundle once this process has gone.
            let app = Bundle.main.bundlePath
            let sh = Process()
            sh.executableURL = URL(fileURLWithPath: "/bin/sh")
            sh.arguments = ["-c", "sleep 1.5; open -n \"$0\"", app]
            try? sh.run()
            resolve(id, true, in: from)
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.2) { NSApp.terminate(nil) }
        default:
            resolve(id, nil, in: from)
        }
    }

    func resolve(_ id: Int, _ value: Any?, in view: WKWebView? = nil) {
        var json = "null"
        if let value = value, let data = try? JSONSerialization.data(withJSONObject: value, options: .fragmentsAllowed),
           let s = String(data: data, encoding: .utf8) {
            json = s
        }
        (view ?? web).evaluateJavaScript("window.lumpyNative.resolve(\(id), \(json))")
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
        win.addItem(item("Show Lumpy", #selector(showWindowAction), "0"))
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
    @objc func showWindowAction() { showWindow() }
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.setActivationPolicy(.regular)
app.run()
