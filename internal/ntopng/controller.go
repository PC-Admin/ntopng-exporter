package ntopng

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/aauren/ntopng-exporter/internal/config"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	luaRestV1Get     = "/lua/rest/v1/get"
	hostCustomFields = `ip,bytes.sent,bytes.rcvd,active_flows.as_client,active_flows.as_server,dns,num_alerts,mac,total_flows.as_client,total_flows.as_server,vlan,total_alerts,name,ifid`
	hostCustomPath      = "/host/custom_data.lua"
	interfaceCustomPath = "/ntopng/interfaces.lua"
)

type Controller struct {
	config   *config.Config
	ifList   map[string]int
	HostList map[string]ntopHost
	HostListMutex *sync.RWMutex
	stopChan <-chan struct{}
}

func CreateController(config *config.Config, stopChan <-chan struct{}) Controller {
	var controller Controller
	controller.config = config
	controller.stopChan = stopChan
	controller.HostListMutex = &sync.RWMutex{}
	return controller
}

func (c *Controller) RunController() {
	scrapeInterval, err := time.ParseDuration(c.config.Ntopng.ScrapeInterval)
	if err != nil {
		fmt.Printf("was not able to parse duration: %s - %v", c.config.Ntopng.ScrapeInterval, err)
		return
	}
	timer := time.NewTimer(scrapeInterval)
	for {
		select {
			case <-timer.C:
				if err := c.ScrapeHostEndpointForAllInterfaces(); err != nil {
					fmt.Printf("encountered an error while scraping interfaces, we were likely stopped short: %v",
						err)
				}
			case <-c.stopChan:
				return
		}
	}
}

func (c *Controller) CacheInterfaceIds() error {
	endpoint := fmt.Sprintf("%s%s%s", c.config.Ntopng.EndPoint, luaRestV1Get, interfaceCustomPath)
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return err
	}
	c.setCommonOptions(req, false)

	body, status, err := getHttpResponseBody(req)
	if status != http.StatusOK {
		return fmt.Errorf("request to interface endpoint was not successful. Status: '%d', Response: '%v'",
			status, *body)
	}

	rawInterfaces, err := getRawJsonFromNtopResponse(body)
	if err != nil {
		return err
	}
	var ifList []ntopInterface
	err = json.Unmarshal(rawInterfaces, &ifList)
	if err != nil {
		return fmt.Errorf("was not able to parse interface list from ntopng: %v", err)
	}
	if len(ifList) < 1 {
		return fmt.Errorf("ntopng returned 0 interfaces: %v", *body)
	}
	c.ifList = make(map[string]int, len(ifList))
	for _, myIf := range ifList {
		c.ifList[myIf.IfName] = myIf.IfID
	}

	for _, configuredIf := range c.config.Host.InterfacesToMonitor {
		if _, ok := c.ifList[configuredIf]; !ok {
			return fmt.Errorf("could not find '%s' interface in list returned by ntopng: %v",
				configuredIf, c.ifList)
		}
	}
	return nil
}

func (c *Controller) ScrapeHostEndpointForAllInterfaces() error {
	// tempNtopHosts is made here to minimize the amount of time we have to lock the list and also to make sure that we
	// don't keep a list of ever growing hosts in our map which could eventually overwhelm the system
	tempNtopHosts := make(map[string]ntopHost)
	for _, configuredIf := range c.config.Host.InterfacesToMonitor {
		if err := c.scrapeHostEndpoint(c.ifList[configuredIf], tempNtopHosts); err != nil {
			return fmt.Errorf("failed to scrape interface '%s' with error: %v", configuredIf, err)
		}
	}
	c.HostListMutex.Lock()
	defer c.HostListMutex.Unlock()
	c.HostList = tempNtopHosts
	return nil
}

func (c *Controller) scrapeHostEndpoint(interfaceId int, tempNtopHosts map[string]ntopHost) error {
	endpoint := fmt.Sprintf("%s%s%s", c.config.Ntopng.EndPoint, luaRestV1Get, hostCustomPath)
	payload := []byte(fmt.Sprintf(`{"ifid": %d, "field_alias": "%s"}`, interfaceId, hostCustomFields))
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	c.setCommonOptions(req, true)

	body, status, err := getHttpResponseBody(req)
	if status != http.StatusOK {
		return fmt.Errorf("request to host endpoint was not successful. Status: '%d', Response: '%v'",
			status, *body)
	}

	rawHosts, err := getRawJsonFromNtopResponse(body)
	if err != nil {
		return err
	}
	var hostList []ntopHost
	err = json.Unmarshal(rawHosts, &hostList)
	if len(hostList) < 1 {
		return fmt.Errorf("ntopng returned 0 hosts: %v", *body)
	}
	var parsedSubnets []*net.IPNet
	if c.config.Metric.LocalSubnetsOnly != nil && len(c.config.Metric.LocalSubnetsOnly) > 0 {
		for _, subnet := range c.config.Metric.LocalSubnetsOnly {
			_, parsedSubnet, _ := net.ParseCIDR(subnet)
			parsedSubnets = append(parsedSubnets, parsedSubnet)
		}
	}
	for _, myHost := range hostList {
		// If we already have this host in our cache and it has a different ifid than we are currently processing, don't
		// overwrite it, and print a warning.
		if err = c.checkForDuplicateInterfaces(&myHost); err != nil {
			fmt.Println(err)
			continue
		}
		if len(parsedSubnets) > 0 {
			validIP := false
			parsedIP := net.ParseIP(myHost.IP)
			for _, parsedSubnet := range parsedSubnets {
				if parsedSubnet.Contains(parsedIP) {
					validIP = true
					break
				}
			}
			if !validIP {
				continue
			}
		}
		if myHost.IfName, err = c.ResolveIfID(myHost.IfID); err != nil {
			fmt.Printf("Could not resolve interface: %d, this should not happen", myHost.IfID)
			myHost.IfName = strconv.Itoa(myHost.IfID)
		}
		tempNtopHosts[myHost.IP] = myHost
	}
	return err
}

func (c *Controller) setCommonOptions(req *http.Request, isJsonRequest bool) {
	if isJsonRequest {
		req.Header.Add("Content-Type", "application/json")
	}
	if c.config.Ntopng.AuthMethod == "cookie" {
		req.Header.Add("Cookie",
			fmt.Sprintf("user=%s; password=%s",
				c.config.Ntopng.User, c.config.Ntopng.Password))
	} else if c.config.Ntopng.AuthMethod == "basic" {
		req.SetBasicAuth(c.config.Ntopng.User, c.config.Ntopng.Password)
	}
}