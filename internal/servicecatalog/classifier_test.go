package servicecatalog

import "testing"

func TestClassifyProviderServices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		courier       string
		code          string
		serviceName   string
		canonicalCode string
		group         string
		serviceType   string
		variant       string
	}{
		{"jne regular variant", "jne", "REG23", "", "REG", GroupRegular, TypeParcel, "REG23"},
		{"jne city yes", "jne", "CTCYES", "JNE City Courier", "YES", GroupNextDay, TypeParcel, "CTCYES"},
		{"jne city super speed", "jne", "CTCSPS", "JNE City Courier", "SPS", GroupExpress, TypeParcel, "CTCSPS"},
		{"jne trucking motor", "jne", "JTR>130", "JNE Trucking", "JTR", GroupCargo, TypeCargo, "JTR>130"},
		{"tiki same day", "tiki", "SDS", "", "SDS", GroupSameDay, TypeSameDay, ""},
		{"tiki trucking", "tiki", "TRUCKING", "", "TRC", GroupCargo, TypeCargo, "TRUCKING"},
		{"tiki motor 150", "tiki", "T15", "Motor Di Bawah 150cc/1500watt", "TRC", GroupCargo, TypeCargo, "T15"},
		{"tiki motor 600", "tiki", "T60", "Motor Di Bawah 600cc/Non Standar/Roda 3", "TRC", GroupCargo, TypeCargo, "T60"},
		{"sicepat regular alias", "sicepat", "REG", "", "REGULER", GroupRegular, TypeParcel, "REG"},
		{"sicepat cargo", "sicepat", "GOKIL", "", "GOKIL", GroupCargo, TypeCargo, ""},
		{"jnt heboh alias", "jnt", "HEBOH", "", "HBO", GroupCargo, TypeCargo, "HEBOH"},
		{"idexpress std", "ide", "STD", "", "REG", GroupRegular, TypeParcel, "STD"},
		{"idexpress truck", "ide", "Idtruck", "Idtruck", "CARGO", GroupCargo, TypeCargo, "Idtruck"},
		{"ninja standard", "ninja", "STANDARD", "", "REG", GroupRegular, TypeParcel, "STANDARD"},
		{"pos regular provider code", "pos", "Pos Reguler", "240", "REGULER", GroupRegular, TypeParcel, "Pos Reguler"},
		{"pos dangerous goods", "pos", "PAKETPOS DANGEROUS GOODS", "Pdg", "DANGEROUS_GOODS", GroupSpecial, TypeParcel, "PAKETPOS DANGEROUS GOODS"},
		{"lion bigpack fast", "lion", "BIGPACK FAST", "", "BIGPACK", GroupCargo, TypeCargo, "BIGPACK FAST"},
		{"pos express next day", "pos", "EXPRESS NEXT DAY BARANG", "", "NEXT_DAY", GroupNextDay, TypeParcel, "EXPRESS NEXT DAY BARANG"},
		{"wahana normal", "wahana", "Normal", "", "EXPRESS", GroupRegular, TypeParcel, "Normal"},
		{"sentral air cargo", "sentral", "AIR CARGO", "", "UDARA", GroupCargo, TypeCargo, "AIR CARGO"},
		{"sap one day", "sap", "ODS", "", "ODS", GroupNextDay, TypeParcel, ""},
		{"sap provider next day", "sap", "UDRONS", "Nextday", "ODS", GroupNextDay, TypeParcel, "UDRONS"},
		{"sap provider cargo", "sap", "DRGREG", "Cargo", "CARGO", GroupCargo, TypeCargo, "DRGREG"},
		{"rex same day alias", "rex", "REX-0", "", "REX0", GroupSameDay, TypeSameDay, ""},
		{"rex cargo ten", "rex", "REX-10", "Rex-10 ( Harga Ekonomis Mulai 10 Kg )", "REX10", GroupCargo, TypeCargo, ""},
		{"anteraja document", "anteraja", "DOK", "Anteraja Document", "DOK", GroupRegular, TypeParcel, ""},
		{"anteraja economy", "anteraja", "ECO", "Anteraja Economy", "ECO", GroupEconomy, TypeParcel, ""},
		{"anteraja mini cargo", "anteraja", "MIC", "Anteraja Mini Cargo", "MIC", GroupCargo, TypeCargo, ""},
		{"anteraja next day", "anteraja", "ND", "Anteraja Next Day", "ND", GroupNextDay, TypeParcel, ""},
		{"dse overnight", "dse", "ONS", "Over Night Service", "ONS", GroupNextDay, TypeParcel, ""},
		{"ncs regular land", "ncs", "DARAT", "Regular Darat", "DARAT", GroupCargo, TypeCargo, ""},
		{"rpx economy", "rpx", "ECP", "Economy Package", "ECP", GroupEconomy, TypeParcel, ""},
		{"rpx pas economy alias", "rpx", "PSC", "PAS Economy", "ECP", GroupEconomy, TypeParcel, "PSC"},
		{"star air cargo", "star", "AIR", "Angkutan Udara", "UDARA", GroupCargo, TypeCargo, "AIR"},
		{"generic standard", "newcourier", "STD", "Standard Service", "STD", GroupRegular, TypeParcel, ""},
		{"generic cargo", "newcourier", "TRUCK", "Heavy Weight", "TRUCK", GroupCargo, TypeCargo, ""},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := Classify(test.courier, test.code, test.serviceName)
			if !got.Matched {
				t.Fatalf("classification did not match: %#v", got)
			}
			if got.CanonicalCode != test.canonicalCode ||
				got.ServiceGroup != test.group ||
				got.ServiceType != test.serviceType ||
				got.VariantCode != test.variant {
				t.Fatalf(
					"got canonical=%q group=%q type=%q variant=%q",
					got.CanonicalCode,
					got.ServiceGroup,
					got.ServiceType,
					got.VariantCode,
				)
			}
		})
	}
}

func TestUnknownProviderServiceRemainsUnknown(t *testing.T) {
	t.Parallel()

	got := Classify("jne", "MYSTERY", "Experimental")
	if got.Matched ||
		got.CanonicalCode != "" ||
		got.ServiceGroup != GroupUnknown ||
		got.ServiceType != TypeUnknown ||
		got.VariantCode != "MYSTERY" {
		t.Fatalf("unexpected unknown classification: %#v", got)
	}
}

func TestRexUnknownNumericServiceDoesNotMatchKnownPrefix(t *testing.T) {
	t.Parallel()

	got := Classify("rex", "REX-100", "Unlisted service")
	if got.Matched || got.ServiceGroup != GroupUnknown {
		t.Fatalf("unexpected REX numeric classification: %#v", got)
	}
}
