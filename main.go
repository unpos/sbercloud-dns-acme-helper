package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"strings"

	"sbercloud-dns-acme-helper/core"

	"github.com/joho/godotenv"
	"golang.org/x/net/publicsuffix"
)

type Projects struct {
	Projects []Project
}

type Project struct {
	Id      string
	Enabled bool
	Name    string
}

type Zones struct {
	Zones []Zone
}

type Zone struct {
	Id     string
	Name   string
	Status string
}

type Records struct {
	Records []Record `json:"recordsets"`
}

type Record struct {
	Id      string
	Name    string
	Type    string
	Records []string
	Status  string
}

func CallApi(method string, endpoint string, projectId string, payload string, s core.Signer) ([]byte, error) {
	r, _ := http.NewRequest(
		method,
		endpoint,
		ioutil.NopCloser(bytes.NewBuffer([]byte(payload))),
	)

	if projectId != "" {
		r.Header.Add("x-project-id", projectId)
	}

	r.Header.Add("content-type", "application/json")
	s.Sign(r)

	client := http.DefaultClient
	resp, err := client.Do(r)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}

func GetProjectId(authEndpoint string, projectName string, s core.Signer) (string, error) {
	var projects Projects
	result, err := CallApi("GET", authEndpoint+"/v3/projects", "", "", s)
	if err != nil {
		return "", err
	}

	json.Unmarshal(result, &projects)

	for i := 0; i < len(projects.Projects); i++ {
		if projects.Projects[i].Enabled && projects.Projects[i].Name == projectName {
			return projects.Projects[i].Id, nil
		}
	}
	return "", fmt.Errorf("no active project named '%s' found", projectName)
}

func GetZoneId(dnsEndpoint string, zoneName string, projectId string, s core.Signer) (string, error) {
	var zones Zones
	result, err := CallApi("GET", dnsEndpoint+"/v2/zones", projectId, "", s)
	if err != nil {
		return "", err
	}

	json.Unmarshal(result, &zones)

	for i := 0; i < len(zones.Zones); i++ {
		if zones.Zones[i].Status == "ACTIVE" && zones.Zones[i].Name == zoneName {
			return zones.Zones[i].Id, nil
		}
	}
	return "", fmt.Errorf("no active '%s' zone found", strings.TrimRight(zoneName, "."))
}

func GetRecordId(dnsEndpoint string, zoneId string, projectId string, fqdn string, challenge string, s core.Signer) (string, error) {
	var records Records
	result, err := CallApi("GET", dnsEndpoint+"/v2/zones/"+zoneId+"/recordsets", projectId, "", s)
	if err != nil {
		return "", err
	}

	json.Unmarshal(result, &records)

	for i := 0; i < len(records.Records); i++ {
		if records.Records[i].Status == "ACTIVE" &&
			records.Records[i].Name == fqdn &&
			records.Records[i].Type == "TXT" &&
			reflect.DeepEqual(records.Records[i].Records, []string{fmt.Sprintf(`"%s"`, challenge)}) {
			return records.Records[i].Id, nil
		}
	}
	return "", fmt.Errorf("no active record '%s' of type 'TXT' with value '%s' found", fqdn, challenge)
}

func Present(dnsEndpoint string, zoneId string, projectId string, fqdn string, challenge string, s core.Signer) (string, error) {
	payload := fmt.Sprintf(`{"name":"%s","type":"TXT","records":["\"%s\""]}`, fqdn, challenge)
	result, err := CallApi("POST", dnsEndpoint+"/v2/zones/"+zoneId+"/recordsets", projectId, payload, s)
	if err != nil {
		return "", err
	}

	return string(result), nil
}

func Cleanup(dnsEndpoint string, zoneId string, projectId string, fqdn string, challenge string, s core.Signer) (string, error) {
	recordId, err := GetRecordId(dnsEndpoint, zoneId, projectId, fqdn, challenge, s)
	if err != nil {
		return "", err
	}

	result, err := CallApi("DELETE", dnsEndpoint+"/v2/zones/"+zoneId+"/recordsets/"+recordId, projectId, "", s)
	if err != nil {
		return "", err
	}

	return string(result), nil
}

func main() {
	godotenv.Load()
	s := core.Signer{
		Key:    os.Getenv("SBC_ACCESS_KEY"),
		Secret: os.Getenv("SBC_SECRET_KEY"),
	}
	projectName := os.Getenv("SBC_PROJECT_NAME")
	authEndpoint := "https://iam." + os.Getenv("SBC_REGION_NAME") + ".hc.sbercloud.ru"
	dnsEndpoint := "https://dns." + os.Getenv("SBC_REGION_NAME") + ".hc.sbercloud.ru"

	var projectId string
	if projectName != "" {
		var err error
		projectId, err = GetProjectId(authEndpoint, projectName, s)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	zoneName, err := publicsuffix.EffectiveTLDPlusOne(
		strings.TrimRight(os.Args[2], "."),
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	zoneName = zoneName + "."
	zoneId, err := GetZoneId(dnsEndpoint, zoneName, projectId, s)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "present":
		_, err := Present(dnsEndpoint, zoneId, projectId, os.Args[2], os.Args[3], s)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	case "cleanup":
		_, err := Cleanup(dnsEndpoint, zoneId, projectId, os.Args[2], os.Args[3], s)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
}
