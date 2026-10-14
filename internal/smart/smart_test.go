package smart

import (
	"context"
	"strings"
	"testing"
)

const ata = `{"json_format_version":[1,0],"smartctl":{"version":[7,4],"exit_status":0},
"device":{"name":"/dev/disk4","type":"ata","protocol":"ATA"},
"model_name":"WDC WD40EFRX-68N32N0","serial_number":"WD-WCC7K1234567",
"smart_status":{"passed":true},"power_on_time":{"hours":12345},"temperature":{"current":34},
"ata_smart_attributes":{"table":[
 {"id":5,"name":"Reallocated_Sector_Ct","raw":{"value":0,"string":"0"}},
 {"id":197,"name":"Current_Pending_Sector","raw":{"value":8,"string":"8"}},
 {"id":198,"name":"Offline_Uncorrectable","raw":{"value":0,"string":"0"}},
 {"id":199,"name":"UDMA_CRC_Error_Count","raw":{"value":3,"string":"3"}}]}}`

const nvme = `{"json_format_version":[1,0],"smartctl":{"version":[7,4],"exit_status":0},
"device":{"name":"/dev/disk0","type":"nvme","protocol":"NVMe"},
"model_name":"APPLE SSD AP2048Z","serial_number":"0ba0",
"smart_status":{"passed":false},"power_on_time":{"hours":900},"temperature":{"current":61},
"nvme_smart_health_information_log":{"critical_warning":0,"temperature":61,"percentage_used":97,"media_errors":2,"power_on_hours":900}}`

const usb = `{"json_format_version":[1,0],"smartctl":{"version":[7,4],"exit_status":2,
"messages":[{"string":"/dev/disk5: Unable to detect device type","severity":"error"}]},
"device":{"name":"/dev/disk5","type":"","protocol":""}}`

func TestParseATA(t *testing.T) {
	h, err := Parse([]byte(ata))
	if err != nil {
		t.Fatal(err)
	}
	if h.Passed == nil || !*h.Passed || h.Bad() || h.Model != "WDC WD40EFRX-68N32N0" || h.Hours != 12345 || h.Temp != 34 {
		t.Fatalf("basics: %+v", h)
	}
	if h.Reallocated != 0 || h.Pending != 8 || h.CRC != 3 {
		t.Fatalf("attributes: %+v", h)
	}
	want := []string{"8 sectors pending reallocation", "3 interface CRC errors: suspect the cable or enclosure"}
	if strings.Join(h.Warnings, "|") != strings.Join(want, "|") {
		t.Fatalf("warnings: %q", h.Warnings)
	}
}

func TestParseNVMe(t *testing.T) {
	h, err := Parse([]byte(nvme))
	if err != nil {
		t.Fatal(err)
	}
	if !h.Bad() || h.PercentUsed != 97 || h.Uncorrect != 2 || h.Temp != 61 || h.Hours != 900 {
		t.Fatalf("nvme: %+v", h)
	}
	if len(h.Warnings) != 4 || h.Warnings[0] != "The drive reports it is failing" || h.Warnings[1] != "2 uncorrectable errors" ||
		h.Warnings[2] != "97% of the rated write life used" || h.Warnings[3] != "Running hot: 61 °C" {
		t.Fatalf("warnings: %q", h.Warnings)
	}
}

func TestParseUnreadable(t *testing.T) {
	if _, err := Parse([]byte(usb)); err == nil || !strings.Contains(err.Error(), "USB enclosures") {
		t.Fatalf("usb: %v", err)
	}
	if _, err := Parse([]byte("not json")); err == nil {
		t.Fatal("garbage parsed")
	}
}

func TestReadWithoutSmartctl(t *testing.T) {
	old := Program
	Program = func() string { return "" }
	defer func() { Program = old }()
	h := Read(context.Background(), "disk4")
	if h.Error != "smartmontools is not installed" || h.ReadAt == 0 {
		t.Fatalf("read: %+v", h)
	}
}
