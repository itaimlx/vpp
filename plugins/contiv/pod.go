// Copyright (c) 2017 Cisco and/or its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package contiv

import (
	"fmt"
	"math/rand"
	"net"
	"os/exec"
	"strconv"
	"strings"

	"github.com/contiv/vpp/plugins/contiv/containeridx/model"
	"github.com/contiv/vpp/plugins/contiv/model/cni"
	linux_intf "github.com/ligato/vpp-agent/plugins/linuxv2/model/interfaces"
	linux_l3 "github.com/ligato/vpp-agent/plugins/linuxv2/model/l3"
	linux_ns "github.com/ligato/vpp-agent/plugins/linuxv2/model/namespace"
	vpp_intf "github.com/ligato/vpp-agent/plugins/vppv2/model/interfaces"
	vpp_l3 "github.com/ligato/vpp-agent/plugins/vppv2/model/l3"
)

const (
	ipv4NetAny = "0.0.0.0/0"
)

// PodConfig groups applied configuration for a container
type PodConfig struct {
	// ID identifies the Pod
	ID string
	// PodName from the CNI request
	PodName string
	// PodNamespace from the CNI request
	PodNamespace string
	// Veth1 one end end of veth pair that is in the given container namespace.
	// Nil if TAPs are used instead.
	Veth1 *linux_intf.LinuxInterface
	// Veth2 is the other end of veth pair in the default namespace
	// Nil if TAPs are used instead.
	Veth2 *linux_intf.LinuxInterface
	// VppIf is AF_PACKET/TAP interface connecting pod to VPP
	VppIf *vpp_intf.Interface
	// PodTap is the host end of the tap connecting pod to VPP
	// Nil if TAPs are not used
	PodTap *linux_intf.LinuxInterface
	// VppARPEntry is ARP entry configured in VPP to route traffic from VPP to pod.
	VppARPEntry *vpp_l3.ARPEntry
	// PodARPEntry is ARP entry configured in the pod to route traffic from pod to VPP.
	PodARPEntry *linux_l3.LinuxStaticARPEntry
	// VppRoute is the route from VPP to the container
	VppRoute *vpp_l3.StaticRoute
	// PodLinkRoute is the route from pod to the default gateway.
	PodLinkRoute *linux_l3.LinuxStaticRoute
	// PodDefaultRoute is the default gateway for the pod.
	PodDefaultRoute *linux_l3.LinuxStaticRoute
}

// podConfigToProto transform config structure to structure that will be persisted
// Beware: Intentionally not all data from config will be persisted, only necessary ones.
func podConfigToProto(cfg *PodConfig) *container.Persisted {
	persisted := &container.Persisted{}
	persisted.ID = cfg.ID
	persisted.PodName = cfg.PodName
	persisted.PodNamespace = cfg.PodNamespace
	if cfg.Veth1 != nil {
		persisted.Veth1Name = cfg.Veth1.Name
	}
	if cfg.Veth2 != nil {
		persisted.Veth2Name = cfg.Veth2.Name
	}
	if cfg.VppIf != nil {
		persisted.VppIfName = cfg.VppIf.Name
	}
	if cfg.PodTap != nil {
		persisted.PodTapName = cfg.PodTap.Name
	}
	if cfg.VppARPEntry != nil {
		persisted.VppARPEntryIP = cfg.VppARPEntry.IpAddress
		persisted.VppARPEntryInterface = cfg.VppARPEntry.Interface
	}
	if cfg.PodARPEntry != nil {
		persisted.PodARPEntryInterface = cfg.PodARPEntry.Interface
		persisted.PodARPEntryIP = cfg.PodARPEntry.IpAddress
	}
	if cfg.VppRoute != nil {
		persisted.VppRouteVrf = cfg.VppRoute.VrfId
		persisted.VppRouteNextHop = cfg.VppRoute.NextHopAddr
		persisted.VppRouteDest = cfg.VppRoute.DstNetwork
	}
	if cfg.PodLinkRoute != nil {
		persisted.PodLinkRouteDest = cfg.PodLinkRoute.DstNetwork
		persisted.PodLinkRouteInterface = cfg.PodLinkRoute.OutgoingInterface
	}
	if cfg.PodDefaultRoute != nil {
		persisted.PodDefaultRouteInterface = cfg.PodDefaultRoute.OutgoingInterface
	}

	return persisted
}

func (s *remoteCNIserver) enableIPv6(request *cni.CNIRequest) error {
	// parse PID from the network namespace
	pid, err := s.getPIDFromNwNsPath(request.NetworkNamespace)
	if err != nil {
		return err
	}

	// execute the sysctl in the namespace of given PID
	cmdStr := fmt.Sprintf("nsenter -t %d -n sysctl net.ipv6.conf.all.disable_ipv6=0", pid)
	s.Logger.Infof("Executing CMD: %s", cmdStr)

	cmdArr := strings.Split(cmdStr, " ")
	cmd := exec.Command("nsenter", cmdArr[1:]...)

	// check the output of the exec
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.Logger.Errorf("CMD exec returned error: %v", err)
		return err
	}
	s.Logger.Infof("CMD output: %s", output)

	return nil
}

// getPIDFromNwNsPath returns PID of the main process of the given network namespace path
func (s *remoteCNIserver) getPIDFromNwNsPath(ns string) (int, error) {
	strArr := strings.Split(ns, "/")
	if len(strArr) == 0 {
		return -1, fmt.Errorf("invalid network namespace - no slash char detected in %s", ns)
	}
	pid := -1
	for _, str := range strArr {
		if i, err := strconv.Atoi(str); err == nil {
			pid = i
			s.Logger.Infof("Container PID derived from NS %s: %d", ns, pid)
			break
		}
	}
	if pid == -1 {
		return -1, fmt.Errorf("unable to detect container PID from NS %s", ns)
	}
	return pid, nil
}

func (s *remoteCNIserver) veth1NameFromRequest(request *cni.CNIRequest) string {
	return trimInterfaceName(request.InterfaceName+request.ContainerId, logicalIfNameMaxLen)
}

func (s *remoteCNIserver) veth1HostIfNameFromRequest(request *cni.CNIRequest) string {
	return request.InterfaceName
}

func (s *remoteCNIserver) veth2NameFromRequest(request *cni.CNIRequest) string {
	return trimInterfaceName(request.ContainerId, logicalIfNameMaxLen)
}

func (s *remoteCNIserver) veth2HostIfNameFromRequest(request *cni.CNIRequest) string {
	return trimInterfaceName(request.ContainerId, linuxIfNameMaxLen)
}

func (s *remoteCNIserver) afpacketNameFromRequest(request *cni.CNIRequest) string {
	return trimInterfaceName(afPacketNamePrefix+s.veth2NameFromRequest(request), logicalIfNameMaxLen)
}

func (s *remoteCNIserver) vppTAPNameFromRequest(request *cni.CNIRequest) string {
	return trimInterfaceName(vppTAPNamePrefix+request.ContainerId, logicalIfNameMaxLen)
}

func (s *remoteCNIserver) linuxTAPNameFromRequest(request *cni.CNIRequest) string {
	return trimInterfaceName(linuxTAPNamePrefix+request.ContainerId, logicalIfNameMaxLen)
}

func (s *remoteCNIserver) tapHostNameFromRequest(request *cni.CNIRequest) string {
	return request.InterfaceName
}

func (s *remoteCNIserver) loopbackNameFromRequest(request *cni.CNIRequest) string {
	return trimInterfaceName("loop"+s.veth2NameFromRequest(request), logicalIfNameMaxLen)
}

func (s *remoteCNIserver) ipAddrForPodVPPIf(podIP string) string {
	tapPrefix, _ := ipv4ToUint32(*s.ipam.VPPIfIPPrefix())

	podAddr, _ := ipv4ToUint32(net.ParseIP(podIP))
	podMask, _ := ipv4ToUint32(net.IP(s.ipam.PodNetwork().Mask))
	podSuffix := podAddr &^ podMask

	tapAddress := uint32ToIpv4(tapPrefix + podSuffix)

	return net.IP.String(tapAddress) + "/32"
}

func (s *remoteCNIserver) hwAddrForContainer() string {
	return "00:00:00:00:00:02"
}

func (s *remoteCNIserver) generateHwAddrForPodVPPIf() string {
	hwAddr := make(net.HardwareAddr, 6)
	rand.Read(hwAddr)
	hwAddr[0] = 2
	hwAddr[1] = 0xfe
	return hwAddr.String()
}

func (s *remoteCNIserver) veth1FromRequest(request *cni.CNIRequest, podIP string) *linux_intf.LinuxInterface {
	return &linux_intf.LinuxInterface{
		Name:        s.veth1NameFromRequest(request),
		Type:        linux_intf.LinuxInterface_VETH,
		Mtu:         s.config.MTUSize,
		Enabled:     true,
		HostIfName:  s.veth1HostIfNameFromRequest(request),
		PhysAddress: s.hwAddrForContainer(),
		IpAddresses: []string{podIP},
		Link: &linux_intf.LinuxInterface_Veth{
			Veth: &linux_intf.LinuxInterface_VethLink{PeerIfName: s.veth2NameFromRequest(request)},
		},
		Namespace: &linux_ns.LinuxNetNamespace{
			Type:      linux_ns.LinuxNetNamespace_NETNS_REF_FD,
			Reference: request.NetworkNamespace,
		},
	}
}

func (s *remoteCNIserver) veth2FromRequest(request *cni.CNIRequest) *linux_intf.LinuxInterface {
	return &linux_intf.LinuxInterface{
		Name:       s.veth2NameFromRequest(request),
		Type:       linux_intf.LinuxInterface_VETH,
		Mtu:        s.config.MTUSize,
		Enabled:    true,
		HostIfName: s.veth2HostIfNameFromRequest(request),
		Link: &linux_intf.LinuxInterface_Veth{
			Veth: &linux_intf.LinuxInterface_VethLink{PeerIfName: s.veth1NameFromRequest(request)},
		},
	}
}

func (s *remoteCNIserver) afpacketFromRequest(request *cni.CNIRequest, podIP string) *vpp_intf.Interface {
	af := &vpp_intf.Interface{
		Name:        s.afpacketNameFromRequest(request),
		Type:        vpp_intf.Interface_AF_PACKET_INTERFACE,
		Mtu:         s.config.MTUSize,
		Enabled:     true,
		Vrf:         s.GetPodVrfID(),
		IpAddresses: []string{s.ipAddrForPodVPPIf(podIP)},
		PhysAddress: s.generateHwAddrForPodVPPIf(),
		Link: &vpp_intf.Interface_Afpacket{
			Afpacket: &vpp_intf.Interface_AfpacketLink{
				HostIfName: s.veth2HostIfNameFromRequest(request),
			},
		},
	}
	return af
}

func (s *remoteCNIserver) tapFromRequest(request *cni.CNIRequest, podIP string) *vpp_intf.Interface {
	tap := &vpp_intf.Interface{
		Name:        s.vppTAPNameFromRequest(request),
		Type:        vpp_intf.Interface_TAP_INTERFACE,
		Mtu:         s.config.MTUSize,
		Enabled:     true,
		Vrf:         s.GetPodVrfID(),
		IpAddresses: []string{s.ipAddrForPodVPPIf(podIP)},
		PhysAddress: s.generateHwAddrForPodVPPIf(),
		Link: &vpp_intf.Interface_Tap{
			Tap: &vpp_intf.Interface_TapLink{},
		},
	}
	if s.tapVersion == 2 {
		tap.GetTap().Version = 2
		tap.GetTap().RxRingSize = uint32(s.tapV2RxRingSize)
		tap.GetTap().TxRingSize = uint32(s.tapV2TxRingSize)
	}
	return tap
}

func (s *remoteCNIserver) podTAP(request *cni.CNIRequest, podIPNet *net.IPNet) *linux_intf.LinuxInterface {
	return &linux_intf.LinuxInterface{
		Name:        s.linuxTAPNameFromRequest(request),
		Type:        linux_intf.LinuxInterface_TAP_TO_VPP,
		Mtu:         s.config.MTUSize,
		Enabled:     true,
		HostIfName:  s.tapHostNameFromRequest(request),
		PhysAddress: s.hwAddrForContainer(),
		IpAddresses: []string{podIPNet.String()},
		Link: &linux_intf.LinuxInterface_Tap{
			Tap: &linux_intf.LinuxInterface_TapLink{
				VppTapIfName: s.vppTAPNameFromRequest(request),
			},
		},
		Namespace: &linux_ns.LinuxNetNamespace{
			Type:      linux_ns.LinuxNetNamespace_NETNS_REF_FD,
			Reference: request.NetworkNamespace,
		},
	}
}

func (s *remoteCNIserver) loopbackFromRequest(request *cni.CNIRequest, loopIP string) *vpp_intf.Interface {
	return &vpp_intf.Interface{
		Name:        s.loopbackNameFromRequest(request),
		Type:        vpp_intf.Interface_SOFTWARE_LOOPBACK,
		Enabled:     true,
		IpAddresses: []string{loopIP},
		Vrf:         s.GetPodVrfID(),
	}
}

func (s *remoteCNIserver) vppRouteFromRequest(request *cni.CNIRequest, podIP string) *vpp_l3.StaticRoute {
	route := &vpp_l3.StaticRoute{
		DstNetwork: podIP,
		VrfId:      s.GetPodVrfID(),
	}
	if s.useTAPInterfaces {
		route.OutgoingInterface = s.vppTAPNameFromRequest(request)
	} else {
		route.OutgoingInterface = s.afpacketNameFromRequest(request)
	}
	return route
}

func (s *remoteCNIserver) vppArpEntry(podIfName string, podIP net.IP, macAddr string) *vpp_l3.ARPEntry {
	return &vpp_l3.ARPEntry{
		Interface:   podIfName,
		IpAddress:   podIP.String(),
		PhysAddress: macAddr,
		Static:      true,
	}
}

func (s *remoteCNIserver) podArpEntry(request *cni.CNIRequest, ifName string, macAddr string) *linux_l3.LinuxStaticARPEntry {
	return &linux_l3.LinuxStaticARPEntry{
		Interface: ifName,
		IpAddress: s.ipam.PodGatewayIP().String(),
		HwAddress: macAddr,
	}
}

func (s *remoteCNIserver) podLinkRouteFromRequest(request *cni.CNIRequest, ifName string) *linux_l3.LinuxStaticRoute {
	return &linux_l3.LinuxStaticRoute{
		OutgoingInterface: ifName,
		Scope:             linux_l3.LinuxStaticRoute_LINK,
		DstNetwork:        s.ipam.PodGatewayIP().String() + "/32",
	}
}

func (s *remoteCNIserver) podDefaultRouteFromRequest(request *cni.CNIRequest, ifName string) *linux_l3.LinuxStaticRoute {
	return &linux_l3.LinuxStaticRoute{
		OutgoingInterface: ifName,
		DstNetwork:        ipv4NetAny,
		Scope:             linux_l3.LinuxStaticRoute_GLOBAL,
		GwAddr:            s.ipam.PodGatewayIP().String(),
	}
}

func trimInterfaceName(name string, maxLen int) string {
	if len(name) > maxLen {
		return name[:maxLen]
	}
	return name
}

// ipv4ToUint32 is simple utility function for conversion between IPv4 and uint32.
func ipv4ToUint32(ip net.IP) (uint32, error) {
	ip = ip.To4()
	if ip == nil {
		return 0, fmt.Errorf("Ip address %v is not ipv4 address (or ipv6 convertible to ipv4 address)", ip)
	}
	var tmp uint32
	for _, bytePart := range ip {
		tmp = tmp<<8 + uint32(bytePart)
	}
	return tmp, nil
}

// uint32ToIpv4 is simple utility function for conversion between IPv4 and uint32.
func uint32ToIpv4(ip uint32) net.IP {
	return net.IPv4(byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip)).To4()
}
