package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
)

const (
	debugLevelCst        = 11
	signalChannelSizeCst = 10

	dirCst = "../../router_state/current/"
	iprCst = "ip_route"
	brCst  = "brctl_show"
	tcCst  = "tc_qdisc_show"

	downloadBandwidthCst = "950Mbit"
	uploadBandwidthCst   = "33Mbit"

	targetRttCst = "30ms"

	cakeInterfaceCommandsFileCst = "cake_interface_commands"
)

var (
	debugLevel uint
)

func main() {

	d := flag.Uint("d", debugLevelCst, "debug level")
	//ipr := flag.String("ipr", iprCst, "ip route")
	br := flag.String("br", brCst, "brctl show")
	tc := flag.String("tc", tcCst, "tc qdisc show")
	dir := flag.String("dir", dirCst, "dir")

	db := flag.String("db", downloadBandwidthCst, "download bandwidth")
	ub := flag.String("ub", uploadBandwidthCst, "upload bandwidth")

	rtt := flag.String("rtt", targetRttCst, "target rtt")

	cicf := flag.String("c", cakeInterfaceCommandsFileCst, "cake interface commands file")

	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.LUTC | log.Lshortfile | log.Lmsgprefix)

	debugLevel = *d

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	complete := make(chan struct{}, signalChannelSizeCst)
	go initSignalHandler(cancel, complete)

	if debugLevel > 10 {
		log.Println("start")
	}

	// iprB := readFile(ctx, *dir, *ipr)
	brB := readFile(ctx, *dir, *br)
	tcB := readFile(ctx, *dir, *tc)

	interfaces := parseBr(brB)
	// parseIpr(iprB)
	codelInterfaces := parseTc(tcB)

	if debugLevel > 10 {
		log.Printf("interfaces: %v", interfaces)
		log.Printf("codelInterfaces: %v", codelInterfaces)
	}

	cakeInterfaceCommands := convertCodelToCake(codelInterfaces, *db, *ub, *rtt)

	if debugLevel > 10 {
		log.Printf("cakeInterfaceCommands: %v", cakeInterfaceCommands)
	}

	insideInterfaceCommands := convertInterfacesToCake(interfaces, *db, *rtt)

	if debugLevel > 10 {
		log.Printf("insideInterfaceCommands: %v", insideInterfaceCommands)
	}

	allCommands := cakeInterfaceCommands + insideInterfaceCommands

	writeFile(ctx, *dir+*cicf, allCommands)

	complete <- struct{}{}

}

func readFile(_ context.Context, dir, file string) []byte {

	b, err := os.ReadFile(dir + file)
	if err != nil {
		log.Fatal(err)
	}

	return b

}

func parseBr(b []byte) []string {
	var interfaces []string
	lines := strings.Split(string(b), "\n")
	for i, line := range lines {
		if debugLevel > 10 {
			log.Printf("line: %v", line)
		}
		if i < 2 {
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := regexp.MustCompile(`\s+`).Split(line, -1)
		if debugLevel > 10 {
			log.Printf("parts: %v", parts)
		}
		interfaces = append(interfaces, parts[len(parts)-1])
	}
	return interfaces
}

func parseTc(b []byte) []string {
	var interfaces []string
	lines := strings.Split(string(b), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "fq_codel") {
			parts := regexp.MustCompile(`\s+`).Split(line, -1)
			if debugLevel > 10 {
				log.Printf("parts: %v", parts)
			}
			interfaces = append(interfaces, parts[4]+" "+parts[5]+" "+parts[6])
		}
	}
	return interfaces
}

func convertCodelToCake(codelInterfaces []string, downloadBandwidth, uploadBandwidth, rtt string) string {
	var sb strings.Builder
	for _, codelInterface := range codelInterfaces {
		sb.WriteString(fmt.Sprintf("tc qdisc del dev %s\n", codelInterface))
		if strings.Contains(codelInterface, "ifb") {
			sb.WriteString(fmt.Sprintf("tc qdisc add dev %s cake bandwidth %s rtt %s\n", codelInterface, uploadBandwidth, rtt))
		} else {
			sb.WriteString(fmt.Sprintf("tc qdisc add dev %s cake bandwidth %s rtt %s\n", codelInterface, downloadBandwidth, rtt))
		}
	}
	return sb.String()
}

func convertInterfacesToCake(interfaces []string, downloadBandwidth, rtt string) string {
	var sb strings.Builder
	for _, interfaceName := range interfaces {
		sb.WriteString(fmt.Sprintf("tc qdisc del dev %s root\n", interfaceName))
		sb.WriteString(fmt.Sprintf("tc qdisc add dev %s root cake bandwidth %s rtt %s\n", interfaceName, downloadBandwidth, rtt))
	}
	return sb.String()
}

// func parseTc(b []byte) {

// )

func writeFile(_ context.Context, filename, cakeInterfaceCommands string) {

	err := os.WriteFile(filename, []byte(cakeInterfaceCommands), 0644)
	if err != nil {
		log.Printf("Failed to write: %v", err)
	} else {
		log.Printf("Wrote %s", filename)
	}

}

// initSignalHandler sets up signal handling for the process, and
// will call cancel() when received
func initSignalHandler(cancel context.CancelFunc, complete <-chan struct{}) {

	c := make(chan os.Signal, signalChannelSizeCst)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	<-c
	log.Printf("Signal caught, closing application")
	cancel()

	log.Printf("Signal caught, cancel() called, and sleeping to allow goroutines to close")

	select {
	case <-complete:
		log.Printf("<-complete exit(0)")
	default:
		log.Printf("Sleep complete, goodbye! exit(0)")
	}

	os.Exit(0)
}
