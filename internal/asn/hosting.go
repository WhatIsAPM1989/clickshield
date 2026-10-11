package asn

import "strconv"

type Network int

const (
	Residential Network = iota
	// Hosting providers rent servers that are widely used as personal VPN
	// exits, so real visitors regularly come from them.
	Hosting
	// Cloud platforms where visitors are almost always automated: crawlers,
	// scrapers and headless browsers run on them, VPN exits rarely do.
	Cloud
)

var cloud = map[uint32]string{
	16509:  "Amazon",
	14618:  "Amazon",
	396982: "Google Cloud",
	15169:  "Google",
	8075:   "Microsoft",
	31898:  "Oracle Cloud",
	36351:  "IBM Cloud",
	45102:  "Alibaba Cloud",
	37963:  "Alibaba Cloud",
	134963: "Alibaba Cloud",
	45090:  "Tencent Cloud",
	132203: "Tencent Cloud",
	55990:  "Huawei Cloud",
	136907: "Huawei Cloud",
	396986: "ByteDance",
}

var hosting = map[uint32]string{
	// Russia and CIS
	210644: "Aeza",
	216071: "VDSina",
	48282:  "VDSina",
	9123:   "Timeweb",
	210976: "Timeweb",
	49505:  "Selectel",
	208677: "Cloud.ru",
	200350: "Yandex Cloud",
	198610: "Beget",
	197695: "REG.RU",
	29182:  "FirstVDS",
	44051:  "Fornex",
	205431: "HostLab",
	57043:  "Hostkey",
	213887: "WAIcore",
	// International
	14061:  "DigitalOcean",
	24940:  "Hetzner",
	213230: "Hetzner",
	16276:  "OVH",
	63949:  "Akamai Linode",
	20473:  "Vultr",
	12876:  "Scaleway",
	51167:  "Contabo",
	197540: "netcup",
	9009:   "M247",
	60068:  "CDN77",
	212238: "Datacamp",
	199524: "G-Core",
	204071: "DEBEE",
	40676:  "Psychz",
	8849:   "Melbicom",
	51765:  "Crea Nova",
	24875:  "NovoServe",
	42532:  "Veesp",
	47583:  "Hostinger",
	57169:  "EDIS",
	63023:  "GTHost",
	62563:  "GTHost",
	36007:  "Kamatera",
	16125:  "Cherry Servers",
	214036: "Ultahost",
	30633:  "Leaseweb",
	60781:  "Leaseweb",
	3223:   "Voxility",
	49981:  "WorldStream",
	39020:  "Comvive",
	7979:   "Servers.com",
	202448: "MVPS",
	12989:  "BlackHOST",
	35415:  "Webzilla",
	53667:  "FranTech",
	8100:   "QuadraNet",
	62240:  "Clouvider",
	137409: "GSL Networks",
	399486: "Virtuo",
	215373: "Delska",
}

// Classify tells what kind of network an autonomous system is.
func Classify(number uint32) Network {
	if _, ok := cloud[number]; ok {
		return Cloud
	}
	if _, ok := hosting[number]; ok {
		return Hosting
	}
	return Residential
}

// Datacenter lists cloud and hosting AS numbers as decimal strings.
func Datacenter() []string {
	out := make([]string, 0, len(cloud)+len(hosting))
	for _, m := range []map[uint32]string{cloud, hosting} {
		for n := range m {
			out = append(out, strconv.FormatUint(uint64(n), 10))
		}
	}
	return out
}
