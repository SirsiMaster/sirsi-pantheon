import SwiftUI

// A loading ScrollView has no useful intrinsic height. Give result sheets a
// usable viewport before their command returns, without constraining embedded
// results or growing the window to the entire report's length.
struct ResultPanelLayout: ViewModifier {
    func body(content: Content) -> some View {
        content.frame(minWidth: 560, idealWidth: 680, minHeight: 460, idealHeight: 640)
    }
}
