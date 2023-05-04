package ironicinspector

const (
	// ServiceAccount -
	ServiceAccount = "ironic-operator-ironic"
	// DatabaseName -
	DatabaseName = "ironic_inspector"
	// IronicInspectorPublicPort -
	IronicInspectorPublicPort int32 = 5050
	// IronicInspectorInternalPort -
	IronicInspectorInternalPort int32 = 5050
	// KollaConfigDbSync -
	KollaConfigDbSync = "/var/lib/config-data/merged/db-sync-config.json"
	// KollaConfig -
	KollaConfig = "/var/lib/config-data/merged/ironic-inspector-config.json"
	// DnsmasqKollaConfig -
	DnsmasqKollaConfig = "/var/lib/config-data/merged/dnsmasq-config.json"
	// HttpbootKollaConfig -
	HttpbootKollaConfig = "/var/lib/config-data/merged/httpboot-config.json"
)
