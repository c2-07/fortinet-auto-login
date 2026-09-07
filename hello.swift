import CoreWLAN

let client = CWWiFiClient.shared()

guard let interface = client.interface() else {
    print("No Wi-Fi interface")
    exit(1)
}

print("Interface:", interface.interfaceName ?? "unknown")
print("Power:", interface.powerOn())

print("SSID:", interface.ssid() ?? "nil")
print("BSSID:", interface.bssid() ?? "nil")
