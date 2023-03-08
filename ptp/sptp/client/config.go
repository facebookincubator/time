/*
Copyright (c) Facebook, Inc. and its affiliates.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"fmt"
	"net"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

// MeasurementConfig describes configuration for how we measure offset
type MeasurementConfig struct {
	PathDelayFilterLength         int           `yaml:"path_delay_filter_length"`          // over how many last path delays we filter
	PathDelayFilter               string        `yaml:"path_delay_filter"`                 // which filter to use, see supported path delay filters const
	PathDelayDiscardFilterEnabled bool          `yaml:"path_delay_discard_filter_enabled"` // controls filter that allows us to discard anomalously small path delays
	PathDelayDiscardBelow         time.Duration `yaml:"path_delay_discard_below"`          // discard path delays that are below this threshold
}

// Validate MeasurementConfig is sane
func (c *MeasurementConfig) Validate() error {
	if c.PathDelayFilterLength < 0 {
		return fmt.Errorf("path_delay_filter_length must be 0 or positive")
	}
	if c.PathDelayFilter != FilterNone && c.PathDelayFilter != FilterMean && c.PathDelayFilter != FilterMedian {
		return fmt.Errorf("path_delay_filter must be either %q, %q or %q", FilterNone, FilterMean, FilterMedian)
	}
	return nil
}

// Config specifies PTPNG run options
type Config struct {
	Iface                    string
	Timestamping             string
	MonitoringPort           int
	Interval                 time.Duration
	ExchangeTimeout          time.Duration
	DSCP                     int
	FirstStepThreshold       time.Duration
	Servers                  map[string]int
	Measurement              MeasurementConfig
	MetricsAggregationWindow time.Duration
	AttemptsTXTS             int
	TimeoutTXTS              time.Duration
	FreeRunning              bool
}

// DefaultConfig returns Config initialized with default values
func DefaultConfig() *Config {
	return &Config{
		Interval:                 time.Second,
		ExchangeTimeout:          100 * time.Millisecond,
		MetricsAggregationWindow: time.Duration(60) * time.Second,
		AttemptsTXTS:             10,
		TimeoutTXTS:              time.Duration(50) * time.Millisecond,
		Timestamping:             HWTIMESTAMP,
	}
}

// Validate config is sane
func (c *Config) Validate() error {
	if c.Interval <= 0 {
		return fmt.Errorf("interval must be greater than zero")
	}
	if c.AttemptsTXTS <= 0 {
		return fmt.Errorf("attemptstxts must be greater than zero")
	}
	if c.TimeoutTXTS <= 0 {
		return fmt.Errorf("timeouttxts must be greater than zero")
	}
	if c.MetricsAggregationWindow <= 0 {
		return fmt.Errorf("metricsaggregationwindow must be greater than zero")
	}
	if c.MonitoringPort < 0 {
		return fmt.Errorf("monitoringport must be 0 or positive")
	}
	if c.DSCP < 0 {
		return fmt.Errorf("dscp must be 0 or positive")
	}
	if c.ExchangeTimeout <= 0 || c.ExchangeTimeout >= c.Interval {
		return fmt.Errorf("exchangetimeout must be greater than zero but less than interval")
	}
	if len(c.Servers) == 0 {
		return fmt.Errorf("at least one server must be specified")
	}
	if c.Timestamping != HWTIMESTAMP && c.Timestamping != SWTIMESTAMP {
		return fmt.Errorf("only %q and %q timestamping is supported", HWTIMESTAMP, SWTIMESTAMP)
	}
	if c.Iface == "" {
		return fmt.Errorf("iface must be specified")
	}
	if err := c.Measurement.Validate(); err != nil {
		return fmt.Errorf("invalid measurement config: %w", err)
	}
	return nil
}

// ReadConfig reads config from the file
func ReadConfig(path string) (*Config, error) {
	c := DefaultConfig()
	cData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(cData, &c)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func addrToIPstr(address string) string {
	if net.ParseIP(address) == nil {
		names, err := net.LookupHost(address)
		if err == nil && len(names) > 0 {
			address = names[0]
		}
	}
	return address
}

// PrepareConfig prepares final version of config based on defaults, CLI flags and on-disk config, and validates resulting config
func PrepareConfig(cfgPath string, targets []string, iface string, monitoringPort int, interval time.Duration, dscp int) (*Config, error) {
	cfg := DefaultConfig()
	var err error
	warn := func(name string) {
		log.Warningf("overriding %s from CLI flag", name)
	}
	if cfgPath != "" {
		cfg, err = ReadConfig(cfgPath)
		if err != nil {
			return nil, fmt.Errorf("reading config from %q: %w", cfgPath, err)
		}
	}
	if len(targets) > 0 {
		warn("targets")
		cfg.Servers = map[string]int{}
		for i, t := range targets {
			address := addrToIPstr(t)
			cfg.Servers[address] = i
		}
	} else {
		newServers := map[string]int{}
		for t, i := range cfg.Servers {
			address := addrToIPstr(t)
			newServers[address] = i
		}
		cfg.Servers = newServers
	}
	if iface != "" && iface != cfg.Iface {
		warn("iface")
		cfg.Iface = iface
	}

	if monitoringPort != 0 && monitoringPort != cfg.MonitoringPort {
		warn("monitoringPort")
		cfg.MonitoringPort = monitoringPort
	}

	if interval != 0 && interval != cfg.Interval {
		warn("interval")
		cfg.Interval = interval
	}

	if dscp != 0 && dscp != cfg.DSCP {
		warn("dscp")
		cfg.DSCP = dscp
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}
	log.Debugf("config: %+v", cfg)
	return cfg, nil
}
