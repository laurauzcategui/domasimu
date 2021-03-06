// Copyright © 2014 Jay R. Wren <jrwren@xmtp.net>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/dnsimple/dnsimple-go/dnsimple"
	"golang.org/x/oauth2"
)

var verbose = flag.Bool("v", false, "Use verbose output")
var list = flag.Bool("l", false, "List domains.")
var update = flag.String("u", "", "Update or create record. The format is 'domain name type oldvalue newvlaue ttl'. Use - for oldvalue to create a new record.")
var del = flag.String("d", "", "Delete record. The format is 'domain name type value'")

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, ".domasimurc config file example:")
		toml.NewEncoder(os.Stderr).Encode(Config{"you@example.com", "TOKENHERE1234"})
	}
	flag.Parse()
	_, token, err := getCreds()
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not read config", err)
		return
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	client := dnsimple.NewClient(tc)

	whoamiResponse, err := client.Identity.Whoami()
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not connect to dnsimple", err)
		return
	}
	accountID := strconv.FormatInt(whoamiResponse.Data.Account.ID, 10)

	if *list {
		domainsResponse, err := client.Domains.ListDomains(accountID, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could get domains %v\n", err)
			return
		}
		for _, domain := range domainsResponse.Data {
			if *verbose {
				fmt.Println(domain.Name, domain.ExpiresOn)
			} else {
				fmt.Println(domain.Name)
			}
		}
		return
	}
	if *update != "" {
		id, err := createOrUpdate(client, *update, accountID)
		if err != nil {
			fmt.Fprintln(os.Stderr, "could not get create or update:", err)
		} else {
			fmt.Printf("record written with id %s\n", id)
		}
		return
	}
	if *del != "" {
		id, err := deleteRecord(client, *del, accountID)
		if err != nil {
			fmt.Fprintln(os.Stderr, "could not delete:", err)
		} else {
			fmt.Printf("record deleted with id %s\n", id)
		}
		return
	}
	for _, domain := range flag.Args() {
		listZoneRecordsResponse, err := client.Zones.ListRecords(accountID, domain, nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, "could not get records:", err)
			continue
		}

		for _, record := range listZoneRecordsResponse.Data {
			if *verbose {
				fmt.Println(record.Name, record.Type, record.Content, record.TTL, record.Priority)
			} else {
				fmt.Println(record.Name, record.Type, record.Content)
			}
		}
	}
}

func getCreds() (string, string, error) {
	configFileName := os.Getenv("DOMASIMU_CONF")
	if configFileName == "" {
		configFileName = filepath.Join(os.Getenv("HOME"), ".domasimurc")
	}
	var config Config
	_, err := toml.DecodeFile(configFileName, &config)
	if err != nil {
		return "", "", err
	}
	return config.User, config.Token, nil
}

type Config struct {
	User  string
	Token string
}

func createOrUpdate(client *dnsimple.Client, message string, accountID string) (string, error) {
	pieces := strings.Split(message, " ")
	if len(pieces) != 6 {
		return "", fmt.Errorf("expected space seperated domain, name, type, oldvalue, newvalue, ttl")
	}

	domain := pieces[0]
	changeRecord := dnsimple.ZoneRecord{
		Name: pieces[1],
		Type: pieces[2],
	}
	oldValue := pieces[3]
	newRecord := changeRecord
	newRecord.Content = pieces[4]
	ttl, _ := strconv.Atoi(pieces[5])
	newRecord.TTL = ttl
	id, err := getRecordIDByValue(client, domain, oldValue, accountID, &changeRecord)

	if err != nil {
		return "", err
	}

	var respID string
	if id == 0 {
		zoneRecordResponse, err := client.Zones.CreateRecord(accountID, domain, newRecord)
		respID = strconv.FormatInt(zoneRecordResponse.Data.ID, 10)

		if err != nil {
			return "", err
		}
	} else {
		zoneRecordResponse, err := client.Zones.UpdateRecord(accountID, domain, id, newRecord)
		respID = strconv.FormatInt(zoneRecordResponse.Data.ID, 10)

		if err != nil {
			return "", err
		}
	}

	return respID, nil
}

func deleteRecord(client *dnsimple.Client, message, accountID string) (string, error) {
	pieces := strings.Split(message, " ")
	if len(pieces) != 4 {
		return "", fmt.Errorf("expected space seperated domain, name, type, value")
	}
	domain := pieces[0]
	changeRecord := dnsimple.ZoneRecord{
		Name: pieces[1],
		Type: pieces[2],
	}
	value := pieces[3]
	id, err := getRecordIDByValue(client, domain, value, accountID, &changeRecord)
	if err != nil {
		return "", err
	}
	if id == 0 {
		return "", fmt.Errorf("could not find record")
	}
	_, err = client.Zones.DeleteRecord(accountID, domain, id)
	respID := strconv.FormatInt(id, 10)

	return respID, err
}

func getRecordIDByValue(client *dnsimple.Client, domain, value, accountID string, changeRecord *dnsimple.ZoneRecord) (int64, error) {
	recordResponse, err := client.Zones.ListRecords(accountID, domain, nil)
	if err != nil {
		return 0, err
	}
	var id int64
	for _, record := range recordResponse.Data {
		if record.Name == changeRecord.Name && record.Type == changeRecord.Type && record.Content == value {
			id = record.ID
			break
		}
	}
	return id, nil
}
