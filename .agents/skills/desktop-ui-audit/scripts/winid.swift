// Prints the on-screen windows of one process as "<CGWindowID>\t<pid>\t<owner>\t<title>",
// largest first, for `screencapture -l`. Usage: winid <pid>
import Cocoa

let wantPid: pid_t = CommandLine.arguments.count > 1 ? pid_t(CommandLine.arguments[1]) ?? 0 : 0
guard wantPid != 0 else {
    FileHandle.standardError.write("usage: winid <pid>\n".data(using: .utf8)!)
    exit(2)
}

let options: CGWindowListOption = [.optionOnScreenOnly, .excludeDesktopElements]
let list = CGWindowListCopyWindowInfo(options, kCGNullWindowID) as? [[String: Any]] ?? []

struct Win { let id: Int; let owner: String; let title: String; let area: Double }
var wins: [Win] = []
for w in list {
    guard (w[kCGWindowOwnerPID as String] as? pid_t) == wantPid,
          (w[kCGWindowLayer as String] as? Int) == 0 else { continue }
    let bounds = w[kCGWindowBounds as String] as? [String: Double] ?? [:]
    wins.append(Win(
        id: w[kCGWindowNumber as String] as? Int ?? 0,
        owner: w[kCGWindowOwnerName as String] as? String ?? "",
        title: w[kCGWindowName as String] as? String ?? "",
        area: (bounds["Width"] ?? 0) * (bounds["Height"] ?? 0)))
}
for w in wins.sorted(by: { $0.area > $1.area }) {
    print("\(w.id)\t\(wantPid)\t\(w.owner)\t\(w.title)")
}
