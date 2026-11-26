// Renders every page of a PDF to PNG with PDFKit (macOS only; no install).
// Run through scripts/pdf-pages.sh, which also handles Linux.
//
//   swift scripts/pdfpages.swift IN.pdf OUT_PREFIX [WIDTH [REDACT...]]
//
// Writes OUT_PREFIX-page1.png, OUT_PREFIX-page2.png, ... Each page is scaled
// to WIDTH pixels (default 1100) on a white background, keeping its own
// orientation, so a landscape page comes out wider than tall. Each REDACT
// string is covered with a grey box wherever it appears, extended to the end
// of its word run (so "4923 N Cali" also hides "4923 N California...").
import AppKit
import PDFKit

let args = CommandLine.arguments
guard args.count >= 3, let doc = PDFDocument(url: URL(fileURLWithPath: args[1])) else {
    FileHandle.standardError.write("usage: pdfpages.swift IN.pdf OUT_PREFIX [WIDTH]\n".data(using: .utf8)!)
    exit(2)
}
let width = args.count > 3 ? Double(args[3]) ?? 1100 : 1100
let redact = args.count > 4 ? Array(args[4...]) : []

// redactBoxes: page-space boxes covering each REDACT match, grown a character
// at a time to the end of its run of text (a table cell, or up to a ":" or "·").
func redactBoxes(_ page: PDFPage) -> [CGRect] {
    var boxes: [CGRect] = []
    for needle in redact {
        for sel in doc.findString(needle, withOptions: .caseInsensitive) where sel.pages.contains(page) {
            var box = sel.bounds(for: page)
            for _ in 0..<60 {
                guard let wider = sel.copy() as? PDFSelection else { break }
                wider.extend(atEnd: 1)
                let text = wider.string ?? ""
                let next = wider.bounds(for: page)
                if let last = text.last, "\n\r\t:·".contains(last) { break }
                if text.count <= (sel.string ?? "").count || next.height > box.height * 1.5 { break }
                sel.extend(atEnd: 1)
                box = next
            }
            boxes.append(box.insetBy(dx: -1, dy: -1))
        }
    }
    return boxes
}

for index in 0..<doc.pageCount {
    guard let page = doc.page(at: index) else { continue }
    let box = page.bounds(for: .mediaBox)
    let rotated = page.rotation % 180 != 0
    let pageW = rotated ? box.height : box.width
    let pageH = rotated ? box.width : box.height
    let scale = width / pageW
    let pixW = Int(width.rounded()), pixH = Int((pageH * scale).rounded())
    guard let ctx = CGContext(
        data: nil, width: pixW, height: pixH, bitsPerComponent: 8, bytesPerRow: 0,
        space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)
    else { exit(1) }
    ctx.setFillColor(CGColor(red: 1, green: 1, blue: 1, alpha: 1))
    ctx.fill(CGRect(x: 0, y: 0, width: pixW, height: pixH))
    ctx.scaleBy(x: scale, y: scale)
    page.draw(with: .mediaBox, to: ctx)
    ctx.setFillColor(CGColor(red: 0.6, green: 0.6, blue: 0.6, alpha: 1))
    for box in redactBoxes(page) {
        ctx.fill(box)
    }
    guard let image = ctx.makeImage(),
        let png = NSBitmapImageRep(cgImage: image).representation(using: .png, properties: [:])
    else { exit(1) }
    let out = "\(args[2])-page\(index + 1).png"
    do {
        try png.write(to: URL(fileURLWithPath: out))
    } catch {
        FileHandle.standardError.write("\(out): \(error)\n".data(using: .utf8)!)
        exit(1)
    }
    print(out)
}
