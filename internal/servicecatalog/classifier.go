package servicecatalog

import (
	"strings"
	"unicode"
)

const (
	GroupEconomy       = "economy"
	GroupRegular       = "regular"
	GroupNextDay       = "next_day"
	GroupExpress       = "express"
	GroupSameDay       = "same_day"
	GroupInstant       = "instant"
	GroupCargo         = "cargo"
	GroupInternational = "international"
	GroupSpecial       = "special"
	GroupUnknown       = "unknown"

	TypeParcel        = "parcel"
	TypeCargo         = "cargo"
	TypeSameDay       = "same_day"
	TypeInstant       = "instant"
	TypeInternational = "international"
	TypeUnknown       = "unknown"
)

type Classification struct {
	CanonicalCode        string
	ServiceGroup         string
	ServiceType          string
	VariantCode          string
	ClassificationSource string
	SourceReference      string
	Matched              bool
}

type rule struct {
	canonicalCode string
	serviceGroup  string
	serviceType   string
	codeEquals    []string
	codePrefixes  []string
	nameContains  []string
	source        string
}

var rulesByCourier = map[string][]rule{
	"anteraja": {
		newExactRule("DOK", GroupRegular, TypeParcel, anterajaSource, []string{"DOK"}, []string{"DOCUMENT", "DOKUMEN"}),
		newExactRule("ECO", GroupEconomy, TypeParcel, anterajaSource, []string{"ECO"}, []string{"ECONOMY"}),
		newExactRule("MIC", GroupCargo, TypeCargo, anterajaSource, []string{"MIC"}, []string{"MINICARGO"}),
		newExactRule("ND", GroupNextDay, TypeParcel, anterajaSource, []string{"ND"}, []string{"NEXTDAY"}),
		newExactRule("REG", GroupRegular, TypeParcel, anterajaSource, []string{"REG"}, []string{"REGULAR"}),
		newExactRule("SD", GroupSameDay, TypeSameDay, anterajaSource, []string{"SD"}, []string{"SAMEDAY"}),
		newExactRule("BIG", GroupCargo, TypeCargo, anterajaSource, []string{"BIG"}, []string{"ANTERAJACARGO"}),
		newExactRule("ICE", GroupSpecial, TypeParcel, anterajaSource, []string{"ICE"}, []string{"FROZEN"}),
	},
	"dse": {
		newRule("SDS", GroupSameDay, TypeSameDay, dseSource, []string{"SDS", "SAMEDAY"}, []string{"SAMEDAYSERVICE"}),
		newRule("ONS", GroupNextDay, TypeParcel, dseSource, []string{"ONS", "OVERNIGHT"}, []string{"OVERNIGHTSERVICE"}),
		newRule("INT", GroupInternational, TypeInternational, dseSource, []string{"INT"}, []string{"INTERNATIONAL"}),
		newRule("CITY", GroupRegular, TypeParcel, dseSource, []string{"CITY"}, []string{"CITYCOURIER"}),
		newRule("REG", GroupRegular, TypeParcel, dseSource, []string{"REG"}, []string{"REGULAR"}),
	},
	"ncs": {
		newRule("SDS", GroupSameDay, TypeSameDay, ncsSource, []string{"SDS", "SAMEDAY"}, []string{"SAMEDAY"}),
		newRule("ONS", GroupNextDay, TypeParcel, ncsSource, []string{"ONS", "OVERNIGHT"}, []string{"ONENIGHT", "OVERNIGHT"}),
		newRule("DARAT", GroupCargo, TypeCargo, ncsSource, []string{"DARAT", "TRUCK", "LTL", "FCL"}, []string{"REGULARDARAT", "TRUCKING"}),
		newRule("NFD", GroupSpecial, TypeParcel, ncsSource, []string{"NFD"}, []string{"NUSANTARAFOODDELIVERY"}),
		newRule("INT", GroupInternational, TypeInternational, ncsSource, []string{"INT"}, []string{"INTERNATIONAL"}),
		newRule("REG", GroupRegular, TypeParcel, ncsSource, []string{"REG"}, []string{"REGULAR"}),
	},
	"rpx": {
		newExactRule("SDP", GroupSameDay, TypeSameDay, rpxSource, []string{"SDP"}, []string{"SAMEDAYPACKAGE"}),
		newExactRule("MDP", GroupNextDay, TypeParcel, rpxSource, []string{"MDP"}, []string{"MIDDAYPACKAGE"}),
		newExactRule("NDP", GroupNextDay, TypeParcel, rpxSource, []string{"NDP"}, []string{"NEXTDAYPACKAGE"}),
		newExactRule("RGP", GroupRegular, TypeParcel, rpxSource, []string{"RGP"}, []string{"REGULARPACKAGE"}),
		newExactRule("HWP", GroupCargo, TypeCargo, rpxSource, []string{"HWP"}, []string{"HEAVYWEIGHTPACKAGE"}),
		newExactRule("ECP", GroupEconomy, TypeParcel, rpxSource, []string{"ECP", "PSC"}, []string{"ECONOMYPACKAGE", "PASECONOMY"}),
		newExactRule("HCP", GroupNextDay, TypeParcel, rpxSource, []string{"HCP", "PHC"}, []string{"HANDCARRYPACKAGE", "PASHANDCARRY"}),
	},
	"star": {
		newRule("UDARA", GroupCargo, TypeCargo, starSource, []string{"UDARA", "AIR"}, []string{"ANGKUTANUDARA"}),
		newRule("LAUT", GroupCargo, TypeCargo, starSource, []string{"LAUT", "SEA"}, []string{"ANGKUTANLAUT"}),
		newRule("DARAT", GroupCargo, TypeCargo, starSource, []string{"DARAT", "LAND"}, []string{"ANGKUTANDARAT"}),
		newRule("CARGO", GroupCargo, TypeCargo, starSource, []string{"CARGO", "KARGO"}, []string{"STARCARGO"}),
	},
	"jne": {
		newRule("YES", GroupNextDay, TypeParcel, jneSource, []string{"CTCYES"}, nil),
		newRule("SPS", GroupExpress, TypeParcel, jneSource, []string{"CTCSPS"}, nil),
		newRule("CTC", GroupRegular, TypeParcel, jneSource, []string{"CTC"}, []string{"CITYCOURIER"}),
		newRule("JTR", GroupCargo, TypeCargo, jneSource, []string{"JTR"}, []string{"JNETRUCKING"}),
		newRule("OKE", GroupEconomy, TypeParcel, jneSource, []string{"OKE"}, nil),
		newRule("YES", GroupNextDay, TypeParcel, jneSource, []string{"YES"}, []string{"YAKINESOKSAMPAI"}),
		newRule("SPS", GroupExpress, TypeParcel, jneSource, []string{"SPS", "SS"}, []string{"SUPERSPEED"}),
		newRule("REG", GroupRegular, TypeParcel, jneSource, []string{"REG"}, []string{"JNEREGULAR", "LAYANANREGULER"}),
		newRule("INT", GroupInternational, TypeInternational, jneSource, []string{"INT"}, []string{"INTERNATIONALEXPRESS"}),
		newRule("DIPLOMAT", GroupSpecial, TypeParcel, jneSource, []string{"DIPLOMAT"}, nil),
		newRule("JESIKA", GroupSpecial, TypeParcel, jneSource, []string{"JESIKA"}, nil),
	},
	"tiki": {
		newRule("SDS", GroupSameDay, TypeSameDay, tikiSource, []string{"SDS"}, []string{"SAMEDAYSERVICE"}),
		newRule("ONS", GroupNextDay, TypeParcel, tikiSource, []string{"ONS"}, []string{"OVERNIGHTSERVICE"}),
		newRule(
			"TRC",
			GroupCargo,
			TypeCargo,
			tikiSource,
			[]string{"TRC", "TRUCKING", "T15", "T25", "T60"},
			[]string{"TRUCKINGSERVICE", "MOTORDIBAWAH"},
		),
		newRule("ECO", GroupEconomy, TypeParcel, tikiSource, []string{"ECO"}, []string{"ECONOMYSERVICE"}),
		newRule("REG", GroupRegular, TypeParcel, tikiSource, []string{"REG"}, []string{"REGULARSERVICE"}),
		newRule("INT", GroupInternational, TypeInternational, tikiSource, []string{"INT"}, []string{"INTERNATIONALSERVICE"}),
		newRule("FROOZY", GroupSpecial, TypeSameDay, tikiSource, []string{"FROOZY"}, []string{"FROZENDELIVERY"}),
		newRule("SRP", GroupSpecial, TypeParcel, tikiSource, []string{"SRP"}, []string{"IKANPRIORITAS"}),
		newRule("DAT", GroupSpecial, TypeParcel, tikiSource, []string{"DAT"}, []string{"TANAMANBUAH"}),
		newRule("TRX", GroupSpecial, TypeParcel, tikiSource, []string{"TRX"}, []string{"REPTIL"}),
	},
	"sicepat": {
		newRule("GOKIL", GroupCargo, TypeCargo, sicepatSource, []string{"GOKIL"}, nil),
		newRule("BEST", GroupNextDay, TypeParcel, sicepatSource, []string{"BEST"}, nil),
		newRule("HALU", GroupEconomy, TypeParcel, sicepatSource, []string{"HALU"}, nil),
		newRule("H3LO", GroupEconomy, TypeParcel, sicepatSource, []string{"H3LO"}, nil),
		newRule("COD", GroupSpecial, TypeParcel, sicepatSource, []string{"COD"}, nil),
		newRule("REGULER", GroupRegular, TypeParcel, sicepatSource, []string{"REG"}, []string{"SICEPATREGULER"}),
	},
	"jnt": {
		newRule("SUPER", GroupExpress, TypeParcel, jntSource, []string{"SUPER"}, []string{"JTSUPER"}),
		newRule("HBO", GroupCargo, TypeCargo, jntSource, []string{"HBO", "HEBOH"}, []string{"JTHEBOH"}),
		newRule("ECO", GroupEconomy, TypeParcel, jntSource, []string{"ECO"}, []string{"JTECO"}),
		newRule("DOC", GroupSpecial, TypeParcel, jntSource, []string{"DOC"}, []string{"JTDOC"}),
		newRule("EZ", GroupRegular, TypeParcel, jntSource, []string{"EZ"}, []string{"JTEZ"}),
	},
	"ide": {
		newRule("SAME_DAY", GroupSameDay, TypeSameDay, idExpressSource, []string{"SAMEDAY"}, []string{"SAMEDAY"}),
		newRule("CARGO", GroupCargo, TypeCargo, idExpressSource, []string{"CARGO", "IDTRUCK"}, []string{"IDEXPRESSCARGO", "IDTRUCK"}),
		newRule("LITE", GroupEconomy, TypeParcel, idExpressSource, []string{"LITE"}, []string{"IDEXPRESSLITE"}),
		newRule("REG", GroupRegular, TypeParcel, idExpressSource, []string{"REG", "STD"}, []string{"IDEXPRESSREGULAR"}),
	},
	"ninja": {
		newRule("SAME_DAY", GroupSameDay, TypeSameDay, ninjaSource, []string{"SAMEDAY"}, []string{"SAMEDAY"}),
		newRule("CROSS_BORDER", GroupInternational, TypeInternational, ninjaSource, []string{"CROSSBORDER"}, []string{"CROSSBORDER"}),
		newRule("FREIGHT", GroupInternational, TypeInternational, ninjaSource, []string{"FREIGHT"}, []string{"FREIGHTFORWARDING"}),
		newRule("B2BR", GroupCargo, TypeCargo, ninjaSource, []string{"B2BR"}, []string{"B2BR"}),
		newRule("CARGO", GroupCargo, TypeCargo, ninjaSource, []string{"CARGO"}, []string{"NINJACARGO"}),
		newRule("COLD", GroupSpecial, TypeParcel, ninjaSource, []string{"COLD"}, []string{"NINJACOLD"}),
		newRule("REG", GroupRegular, TypeParcel, ninjaSource, []string{"REG", "STD", "STANDARD"}, []string{"NINJAREGULAR", "STANDARDDELIVERY"}),
	},
	"lion": {
		newRule("BIGPACK", GroupCargo, TypeCargo, lionSource, []string{"BIGPACK"}, []string{"BIGPACK"}),
		newRule("INTERPACK", GroupInternational, TypeInternational, lionSource, []string{"INTERPACK"}, []string{"INTERPACK"}),
		newRule("BOSSPACK", GroupExpress, TypeParcel, lionSource, []string{"BOSSPACK"}, []string{"BOSSPACK"}),
		newRule("JAGOPACK", GroupEconomy, TypeParcel, lionSource, []string{"JAGOPACK"}, []string{"JAGOPACK"}),
		newRule("REGPACK", GroupRegular, TypeParcel, lionSource, []string{"REGPACK"}, []string{"REGPACK"}),
	},
	"pos": {
		newRule("DANGEROUS_GOODS", GroupSpecial, TypeParcel, posSource, []string{"PAKETPOSDANGEROUSGOODS"}, []string{"DANGEROUSGOODS"}),
		newRule("VALUABLE_GOODS", GroupSpecial, TypeParcel, posSource, []string{"PAKETPOSVALUABLEGOODS"}, []string{"VALUABLEGOODS"}),
		newRule("E_PACKET", GroupInternational, TypeInternational, posSource, []string{"EPACKET"}, []string{"EPACKET"}),
		newRule("POS_EKSPOR", GroupInternational, TypeInternational, posSource, []string{"POSEKSPOR"}, []string{"POSEKSPOR"}),
		newRule("EMS", GroupInternational, TypeInternational, posSource, []string{"EMS"}, []string{"EXPRESSMAILSERVICE"}),
		newRule("SAME_DAY", GroupSameDay, TypeSameDay, posSource, []string{"SAMEDAY", "Q9"}, []string{"POSSAMEDAY"}),
		newRule("NEXT_DAY", GroupNextDay, TypeParcel, posSource, []string{"NEXTDAY", "EXPRESSNEXTDAY"}, []string{"POSNEXTDAY", "EXPRESSNEXTDAY"}),
		newRule("KARGO", GroupCargo, TypeCargo, posSource, []string{"KARGO", "CARGO"}, []string{"POSKARGO"}),
		newRule("EKONOMI", GroupEconomy, TypeParcel, posSource, []string{"EKONOMI"}, []string{"POSEKONOMI"}),
		newRule("REGULER", GroupRegular, TypeParcel, posSource, []string{"REG", "PKH", "POSREGULER"}, []string{"POSREGULER", "PAKETKILATKHUSUS"}),
	},
	"wahana": {
		newRule("INTERNASIONAL", GroupInternational, TypeInternational, wahanaSource, []string{"INT"}, []string{"INTERNASIONAL"}),
		newRule("NEXT_DAY", GroupNextDay, TypeParcel, wahanaSource, []string{"NEXTDAY"}, []string{"NEXTDAY"}),
		newRule("KARGO", GroupCargo, TypeCargo, wahanaSource, []string{"KARGO", "CARGO"}, []string{"WAHANAKARGO"}),
		newRule("EKONOMIS", GroupEconomy, TypeParcel, wahanaSource, []string{"EKONOMIS", "ECONOMY"}, []string{"WAHANAEKONOMIS"}),
		newRule("COD", GroupSpecial, TypeParcel, wahanaSource, []string{"COD"}, []string{"CASHONDELIVERY"}),
		newRule("EXPRESS", GroupRegular, TypeParcel, wahanaSource, []string{"EXPRESS", "NORMAL"}, []string{"WAHANAEXPRESS"}),
	},
	"sentral": {
		newRule("UDARA", GroupCargo, TypeCargo, sentralSource, []string{"UDARA", "AIR"}, []string{"CARGOUDARA"}),
		newRule("LAUT", GroupCargo, TypeCargo, sentralSource, []string{"LAUT", "SEA"}, []string{"CARGOLAUT"}),
		newRule("DARAT", GroupCargo, TypeCargo, sentralSource, []string{"DARAT", "LAND"}, []string{"CARGODARAT"}),
	},
	"sap": {
		newRule("SDS", GroupSameDay, TypeSameDay, sapSource, []string{"SDS"}, []string{"SAMEDAY"}),
		newRule("ODS", GroupNextDay, TypeParcel, sapSource, []string{"ODS", "UDRONS"}, []string{"ONEDAY", "NEXTDAY"}),
		newRule("INT", GroupInternational, TypeInternational, sapSource, []string{"INT"}, []string{"INTERNATIONAL"}),
		newRule("DEDICATED", GroupSpecial, TypeParcel, sapSource, []string{"DEDICATED"}, []string{"DEDICATEDCOURIER"}),
		newRule("CARGO", GroupCargo, TypeCargo, sapSource, []string{"CARGO", "KARGO", "DARAT", "LAUT", "UDARA", "DRGREG"}, []string{"SAPXCARGO"}),
		newRule("REG", GroupRegular, TypeParcel, sapSource, []string{"REG", "UDRREG"}, []string{"SAPXREGULAR", "REGULER"}),
	},
	"rex": {
		newExactRule("REX10", GroupCargo, TypeCargo, rajaOngkirQuoteSource, []string{"REX10"}, []string{"EKONOMISMULAI10KG"}),
		newExactRule("REX0", GroupSameDay, TypeSameDay, rexSource, []string{"REX0", "SDS"}, []string{"SAMEDAYSERVICE"}),
		newExactRule("REX1", GroupNextDay, TypeParcel, rexSource, []string{"REX1", "ONS"}, []string{"OVERNIGHTSERVICE"}),
		newRule("INT", GroupInternational, TypeInternational, rexSource, []string{"INT"}, []string{"REXINTERNATIONAL"}),
		newRule("EXP", GroupExpress, TypeParcel, rexSource, []string{"EXP"}, []string{"EXPRESSSERVICE"}),
		newRule("REG", GroupRegular, TypeParcel, rexSource, []string{"REG"}, []string{"REXREGULAR"}),
		newRule("OTH", GroupSpecial, TypeCargo, rexSource, []string{"OTH"}, []string{"OTHER"}),
	},
}

const (
	anterajaSource        = "https://anteraja.id/id/services"
	dseSource             = "https://www.21express.co.id/layanan-kami"
	ncsSource             = "https://ncskurir.com/ncskurir/product-service"
	rpxSource             = "https://www.rpx.co.id/service/domestic-express-id"
	starSource            = "https://starcargo.co.id/pages/index/tentang-kami"
	jneSource             = "https://www.jne.co.id/produk-dan-layanan"
	tikiSource            = "https://www.tiki.id/id/produk"
	sicepatSource         = "https://ekspres.sicepat.com/services/reguler"
	jntSource             = "https://www.jet.co.id/information/terms/conditions"
	idExpressSource       = "https://idexpress.com/"
	ninjaSource           = "https://www.ninjaxpress.co/id-id"
	lionSource            = "https://erp.lionparcel.com/"
	posSource             = "https://www.posindonesia.co.id/id/pages/pos-reguler"
	wahanaSource          = "https://wahana.com/syarat-ketentuan"
	sentralSource         = "https://sentralcargo.co.id/syarat-dan-ketentuan"
	sapSource             = "https://www.sapx.id/id"
	rexSource             = "https://www.rex.co.id/en/busines-opportunities/2/to-be-our-cash-sales-counter"
	rajaOngkirQuoteSource = "https://rajaongkir.com/docs/shipping-cost/endpoint-rajaongkir-for-search-base/calculate-domestic-cost"
)

func newExactRule(
	canonicalCode string,
	serviceGroup string,
	serviceType string,
	source string,
	codeEquals []string,
	nameContains []string,
) rule {
	return rule{
		canonicalCode: canonicalCode,
		serviceGroup:  serviceGroup,
		serviceType:   serviceType,
		codeEquals:    codeEquals,
		nameContains:  nameContains,
		source:        source,
	}
}

func newRule(
	canonicalCode string,
	serviceGroup string,
	serviceType string,
	source string,
	codePrefixes []string,
	nameContains []string,
) rule {
	return rule{
		canonicalCode: canonicalCode,
		serviceGroup:  serviceGroup,
		serviceType:   serviceType,
		codePrefixes:  codePrefixes,
		nameContains:  nameContains,
		source:        source,
	}
}

func Classify(courierCode, rawCode, rawName string) Classification {
	codeToken := normalizeToken(rawCode)
	nameToken := normalizeToken(rawName)
	for _, candidate := range rulesByCourier[strings.ToLower(strings.TrimSpace(courierCode))] {
		if !candidate.matches(codeToken, nameToken) {
			continue
		}
		variantCode := ""
		if codeToken != normalizeToken(candidate.canonicalCode) {
			variantCode = strings.TrimSpace(rawCode)
		}
		return Classification{
			CanonicalCode:        candidate.canonicalCode,
			ServiceGroup:         candidate.serviceGroup,
			ServiceType:          candidate.serviceType,
			VariantCode:          variantCode,
			ClassificationSource: "inferred_rule",
			SourceReference:      candidate.source,
			Matched:              true,
		}
	}
	if serviceGroup, serviceType, matched := classifyGenericService(codeToken, nameToken); matched {
		return Classification{
			CanonicalCode:        strings.ToUpper(strings.TrimSpace(rawCode)),
			ServiceGroup:         serviceGroup,
			ServiceType:          serviceType,
			ClassificationSource: "inferred_rule",
			SourceReference:      rajaOngkirQuoteSource,
			Matched:              true,
		}
	}
	return Classification{
		ServiceGroup:         GroupUnknown,
		ServiceType:          TypeUnknown,
		VariantCode:          strings.TrimSpace(rawCode),
		ClassificationSource: "provider_observed",
		Matched:              false,
	}
}

func classifyGenericService(codeToken, nameToken string) (string, string, bool) {
	combined := codeToken + " " + nameToken
	containsAny := func(values ...string) bool {
		for _, value := range values {
			if strings.Contains(combined, value) {
				return true
			}
		}
		return false
	}
	switch {
	case containsAny("CARGO", "KARGO", "TRUCK", "HEAVYWEIGHT", "MINICARGO"):
		return GroupCargo, TypeCargo, true
	case containsAny("SAMEDAY") || codeToken == "SD" || codeToken == "SDS":
		return GroupSameDay, TypeSameDay, true
	case containsAny("NEXTDAY", "ONEDAY", "OVERNIGHT") ||
		codeToken == "ND" || codeToken == "NDP" || codeToken == "ONS":
		return GroupNextDay, TypeParcel, true
	case containsAny("ECONOMY", "ECONOMIC", "EKONOMI", "HEMAT") ||
		codeToken == "ECO" || codeToken == "EKO" || codeToken == "ECP":
		return GroupEconomy, TypeParcel, true
	case containsAny("EXPRESS") || codeToken == "EXP":
		return GroupExpress, TypeParcel, true
	case containsAny("REGULAR", "REGULER", "STANDARD", "NORMAL", "DOCUMENT", "DOKUMEN") ||
		codeToken == "REG" || codeToken == "RGP" || codeToken == "STD" || codeToken == "DOK":
		return GroupRegular, TypeParcel, true
	default:
		return "", "", false
	}
}

func (r rule) matches(codeToken, nameToken string) bool {
	for _, exact := range r.codeEquals {
		if codeToken == normalizeToken(exact) {
			return true
		}
	}
	for _, prefix := range r.codePrefixes {
		if strings.HasPrefix(codeToken, normalizeToken(prefix)) {
			return true
		}
	}
	for _, fragment := range r.nameContains {
		if strings.Contains(nameToken, normalizeToken(fragment)) {
			return true
		}
	}
	return false
}

func normalizeToken(value string) string {
	var token strings.Builder
	for _, char := range strings.ToUpper(strings.TrimSpace(value)) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			token.WriteRune(char)
		}
	}
	return token.String()
}
