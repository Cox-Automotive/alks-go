// Test fixture shaped like a Zscaler local-proxy PAC. No real hostnames are resolved:
// every rule here works from the host string or a literal IP address.
function FindProxyForURL(url, host) {
  if (isPlainHostName(host)) return "DIRECT";
  if (shExpMatch(host, "login.microsoftonline.com")) return "DIRECT";
  if (dnsDomainIs(host, "list.example.com")) return "PROXY 127.0.0.1:9000; DIRECT";
  // isInNet resolves names through DNS, so only call it for literal addresses.
  if (/^\d+\.\d+\.\d+\.\d+$/.test(host) && isInNet(host, "10.0.0.0", "255.0.0.0")) return "DIRECT";
  if (dnsDomainIs(host, ".coxautoinc.com")) return "PROXY 127.0.0.1:9000";
  return "PROXY 127.0.0.1:9000";
}
