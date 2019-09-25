// +build linux

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	dhcp "github.com/krolaw/dhcp4"
	"github.com/krolaw/dhcp4/conn"
	"gitlab.com/bobbae/proxy"
	"gitlab.com/bobbae/q"

	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"

	toml "github.com/pelletier/go-toml"
)

//DHCPHandler holds options for DHCP service handling
type DHCPHandler struct {
	IP            net.IP        // Server IP to use
	Options       dhcp.Options  // Options to send to DHCP Clients
	Start         net.IP        // Start of IP range to distribute
	LeaseRange    int           // Number of IPs to distribute (starting from start)
	LeaseDuration time.Duration // Lease period
	Leases        map[int]lease // Map to keep track of leases
}

var gHandler *DHCPHandler

//Configuration for various features
type Configuration struct {
	DHCP    DHCPConfiguration
	Proxy   ProxyConfiguration
	RESTAPI RESTAPIConfiguration
}

//DHCPConfiguration per interface
type DHCPConfiguration struct {
	Enabled                                                          bool
	ServerIP, RouterIP, NameServerIP, SubnetMask, Interface, StartIP string
	Duration, LeaseRange                                             int64
}

//ProxyConfiguration for initial proxy
type ProxyConfiguration struct {
	Enabled                                    bool
	LocalAddr, RemoteAddr, LocalCert, LocalKey string
	LocalTLS, RemoteTLS                        bool
}

//RESTAPIConfiguration holds config for webserver app with REST API
type RESTAPIConfiguration struct {
	Enabled bool
}

//Config holds global config
var Config Configuration

func main() {
	configFile := flag.String("config", "config.toml", "config file")

	flag.Parse()

	cdata, err := ioutil.ReadFile(*configFile)

	//config, err := toml.LoadFile(*configFile)

	if err != nil {
		log.Fatalf("error reading config file, %s, %v", *configFile, err)
	}

	//serverIP := config.Get("dhcp.server-ip").(string)

	if err := toml.Unmarshal(cdata, &Config); err != nil {
		log.Fatalf("Cannot parse config file")
	}

	if err := checkConfig(); err != nil {
		log.Fatalf("error in configuration, %v", err)
	}

	if Config.DHCP.Enabled {
		go doDHCP()
	}

	if Config.Proxy.Enabled {
		go proxyServer()
	}

	if Config.RESTAPI.Enabled {
		go restAPIServer()
	}

	waitForExit()
}

func proxyServer() {
	laddr, err := net.ResolveTCPAddr("tcp", Config.Proxy.LocalAddr)
	if err != nil {
		log.Fatalf("cannot resolve local addr %s, %v", Config.Proxy.LocalAddr, err)
	}
	raddr, err := net.ResolveTCPAddr("tcp", Config.Proxy.RemoteAddr)
	if err != nil {
		log.Fatalf("cannot resolve remote addr %s, %v", Config.Proxy.RemoteAddr, err)
	}

	if Config.Proxy.LocalTLS {
		if !exists(Config.Proxy.LocalCert) {
			log.Fatalf("certificate file %s does not exist", Config.Proxy.LocalCert)
		}

		if !exists(Config.Proxy.LocalKey) {
			log.Fatalf("key file %s does not exist", Config.Proxy.LocalKey)
		}
	}

	var p = new(proxy.Server)
	if Config.Proxy.RemoteTLS {
		// Testing only. You needs to specify config.ServerName insteand of InsecureSkipVerify
		p = proxy.NewServer(raddr, nil, &tls.Config{InsecureSkipVerify: true})
	} else {
		p = proxy.NewServer(raddr, nil, nil)
	}

	log.Printf("Proxying from %s to %s\n", Config.Proxy.LocalAddr, p.Target.String())
	if Config.Proxy.LocalTLS {
		p.ListenAndServeTLS(laddr, Config.Proxy.LocalCert, Config.Proxy.LocalKey)
	} else {
		p.ListenAndServe(laddr)
	}
}

func exists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

func doDHCP() {
	handler := &DHCPHandler{
		IP:            net.ParseIP(Config.DHCP.ServerIP),
		LeaseDuration: time.Duration(Config.DHCP.Duration) * time.Hour,
		Start:         net.ParseIP(Config.DHCP.StartIP),
		LeaseRange:    int(Config.DHCP.LeaseRange),
		Leases:        make(map[int]lease, Config.DHCP.LeaseRange+1),
		Options: dhcp.Options{
			dhcp.OptionSubnetMask:       []byte(net.ParseIP(Config.DHCP.SubnetMask)),
			dhcp.OptionRouter:           []byte(net.ParseIP(Config.DHCP.RouterIP)),
			dhcp.OptionDomainNameServer: []byte(net.ParseIP(Config.DHCP.NameServerIP)),
		},
	}

	gHandler = handler

	q.Q("dhcp server start", Config.DHCP, handler)
	c, err := conn.NewUDP4BoundListener(Config.DHCP.Interface, ":67")
	if err != nil {
		log.Fatalf("can't create udp listener, %v", err)
	}
	q.Q("dhcp server listening on", c)
	err = dhcp.Serve(c, handler) //Linux
	//log.Fatal(dhcp.Serve(dhcp.NewUDP4FilterListener("en0",":67"), handler)) //MacOSX, etc.
	if err != nil {
		log.Fatalf("dhcp server error, %v", err)
	}
}

func checkConfig() error {
	q.Q("config", Config)
	if Config.DHCP.Enabled {
		if Config.DHCP.ServerIP == "" || Config.DHCP.StartIP == "" || Config.DHCP.RouterIP == "" ||
			Config.DHCP.NameServerIP == "" || Config.DHCP.Interface == "" {
			return fmt.Errorf("missing config parameter(s)")
		}
		if Config.DHCP.Duration <= 0 || Config.DHCP.LeaseRange <= 0 {
			return fmt.Errorf("invalid parameter(s)")
		}
	}

	if Config.Proxy.Enabled {
	}

	return nil
}

func waitForExit() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	<-c
	os.Exit(0)
}

type lease struct {
	nic    string    // Client's CHAddr
	expiry time.Time // When the lease expires
}

//ServeDHCP processes incoming DHCP packets
func (h *DHCPHandler) ServeDHCP(p dhcp.Packet, msgType dhcp.MessageType, options dhcp.Options) (d dhcp.Packet) {
	switch msgType {

	case dhcp.Discover:
		q.Q("dhcp discover", p)
		free, nic := -1, p.CHAddr().String()
		for i, v := range h.Leases { // Find previous lease
			if v.nic == nic {
				free = i
				goto reply
			}
		}
		if free = h.freeLease(); free == -1 {
			return
		}
	reply:
		return dhcp.ReplyPacket(p, dhcp.Offer, h.IP, dhcp.IPAdd(h.Start, free), h.LeaseDuration,
			h.Options.SelectOrderOrAll(options[dhcp.OptionParameterRequestList]))

	case dhcp.Request:
		q.Q("dhcp request", p)
		if server, ok := options[dhcp.OptionServerIdentifier]; ok && !net.IP(server).Equal(h.IP) {
			q.Q("spurious")
			return nil
		}
		reqIP := net.IP(options[dhcp.OptionRequestedIPAddress])
		if reqIP == nil {
			reqIP = net.IP(p.CIAddr())
		}
		q.Q("reqIP", reqIP)
		if len(reqIP) == 4 && !reqIP.Equal(net.IPv4zero) {
			if leaseNum := dhcp.IPRange(h.Start, reqIP) - 1; leaseNum >= 0 && leaseNum < h.LeaseRange {
				if l, exists := h.Leases[leaseNum]; !exists || l.nic == p.CHAddr().String() {
					h.Leases[leaseNum] = lease{nic: p.CHAddr().String(), expiry: time.Now().Add(h.LeaseDuration)}
					p.SetGIAddr(net.IP(h.Options[dhcp.OptionRouter]))
					pkt := dhcp.ReplyPacket(p, dhcp.ACK, h.IP, reqIP, h.LeaseDuration,
						h.Options.SelectOrderOrAll(options[dhcp.OptionParameterRequestList]))
					q.Q("ACK reply", pkt, h.IP, reqIP)
					return pkt
				}
			}
		}
		pkt := dhcp.ReplyPacket(p, dhcp.NAK, h.IP, nil, 0, nil)
		q.Q("NAK reply", pkt)
		return pkt

	case dhcp.Release, dhcp.Decline:
		q.Q("dhcp", msgType, p)
		nic := p.CHAddr().String()
		for i, v := range h.Leases {
			if v.nic == nic {
				delete(h.Leases, i)
				break
			}
		}
	}
	return nil
}

func (h *DHCPHandler) freeLease() int {
	now := time.Now()
	b := rand.Intn(h.LeaseRange) // Try random first
	for _, v := range [][]int{[]int{b, h.LeaseRange}, []int{0, b}} {
		for i := v[0]; i < v[1]; i++ {
			if l, ok := h.Leases[i]; !ok || l.expiry.Before(now) {
				return i
			}
		}
	}
	return -1
}

type (
	user struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
)

var (
	users = map[int]*user{}
	seq   = 1
)

func createUser(c echo.Context) error {
	u := &user{
		ID: seq,
	}
	if err := c.Bind(u); err != nil {
		return err
	}
	users[u.ID] = u
	seq++
	return c.JSON(http.StatusCreated, u)
}

func getUser(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	return c.JSON(http.StatusOK, users[id])
}

func updateUser(c echo.Context) error {
	u := new(user)
	if err := c.Bind(u); err != nil {
		return err
	}
	id, _ := strconv.Atoi(c.Param("id"))
	users[id].Name = u.Name
	return c.JSON(http.StatusOK, users[id])
}

func deleteUser(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	delete(users, id)
	return c.NoContent(http.StatusNoContent)
}

func dumpDHCP(c echo.Context) error {
	q.Q("dumping dhcp", gHandler)
	return c.JSON(http.StatusOK, gHandler)
}

func restAPIServer() {
	e := echo.New()

	e.HideBanner = true

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Routes
	e.POST("/users", createUser)
	e.GET("/users/:id", getUser)
	e.PUT("/users/:id", updateUser)
	e.DELETE("/users/:id", deleteUser)

	e.GET("/dhcp/dump", dumpDHCP)

	// Start server
	e.Logger.Fatal(e.Start(":2345"))
}
