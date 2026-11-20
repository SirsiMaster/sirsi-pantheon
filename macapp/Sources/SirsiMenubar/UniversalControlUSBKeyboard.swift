import Foundation
import IOKit

enum UniversalControlUSBKeyboard {
    // This is a point-in-time registry inventory, not proof that keys work.
    static func inspect() -> [String]? {
        var iterator: io_iterator_t = 0
        guard IOServiceGetMatchingServices(kIOMainPortDefault, IOServiceMatching("IOHIDDevice"), &iterator) == KERN_SUCCESS else { return nil }
        defer { IOObjectRelease(iterator) }
        func property(_ entry: io_registry_entry_t, _ key: String) -> Any? {
            IORegistryEntryCreateCFProperty(entry, key as CFString, kCFAllocatorDefault, 0)?.takeRetainedValue()
        }
        var names = Set<String>()
        while case let service = IOIteratorNext(iterator), service != 0 {
            defer { IOObjectRelease(service) }
            if property(service, "PrimaryUsagePage") as? Int == 1,
               property(service, "PrimaryUsage") as? Int == 6,
               property(service, "Transport") as? String == "USB",
               let name = property(service, "Product") as? String, !name.isEmpty {
                names.insert(name)
            }
        }
        return names.sorted()
    }
}
