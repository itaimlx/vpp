// Copyright (c) 2018 Cisco and/or its affiliates.
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

package ipv4net

import (
	"github.com/ligato/cn-infra/config"
	"github.com/ligato/cn-infra/logging"
	"github.com/ligato/cn-infra/rpc/rest"
	"github.com/ligato/cn-infra/servicelabel"
	"github.com/ligato/vpp-agent/plugins/govppmux"
)

const (
	// ConfigFlagName is name of flag that can be used to define config for the Contiv agent
	ConfigFlagName = "contiv"

	// ContivConfigPath is the default location of Agent's Contiv plugin. This path reflects configuration in k8s/contiv-vpp.yaml.
	ContivConfigPath = "/etc/agent/contiv.yaml"

	// ContivConfigPathUsage explains the purpose of 'kube-config' flag.
	ContivConfigPathUsage = "Path to the Agent's Contiv plugin configuration yaml file."
)

// NewPlugin creates a new Plugin with the provides Options
func NewPlugin(opts ...Option) *IPv4Net {
	p := &IPv4Net{}

	p.PluginName = "ipv4net"
	p.ServiceLabel = &servicelabel.DefaultPlugin
	p.GoVPP = &govppmux.DefaultPlugin
	p.HTTPHandlers = &rest.DefaultPlugin

	for _, o := range opts {
		o(p)
	}

	if p.Deps.Log == nil {
		p.Deps.Log = logging.ForPlugin(p.String())
	}

	if p.Deps.Cfg == nil {
		p.Deps.Cfg = config.ForPlugin(p.String(), config.WithCustomizedFlag(ConfigFlagName, ContivConfigPath, ContivConfigPathUsage))
	}
	return p
}

// Option is a function that acts on a Plugin to inject Dependencies or configuration
type Option func(*IPv4Net)

// UseDeps returns Option that can inject custom dependencies.
func UseDeps(cb func(*Deps)) Option {
	return func(p *IPv4Net) {
		cb(&p.Deps)
	}
}