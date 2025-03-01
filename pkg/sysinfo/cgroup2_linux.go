/*
   Copyright The containerd Authors.

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

/*
   Portions from https://github.com/moby/moby/blob/cff4f20c44a3a7c882ed73934dec6a77246c6323/pkg/sysinfo/cgroup2_linux.go
   Copyright (C) Docker/Moby authors.
   Licensed under the Apache License, Version 2.0
   NOTICE: https://github.com/moby/moby/blob/cff4f20c44a3a7c882ed73934dec6a77246c6323/NOTICE
*/

package sysinfo // import "github.com/docker/docker/pkg/sysinfo"

import (
	"context"
	"os"
	"path"
	"strings"

	"github.com/containerd/cgroups/v3"
	cgroupsV2 "github.com/containerd/cgroups/v3/cgroup2"
	"github.com/containerd/log"
	"github.com/moby/sys/userns"
)

func newV2(options ...Opt) *SysInfo {
	sysInfo := &SysInfo{
		CgroupUnified: true,
		cg2GroupPath:  "/",
	}
	for _, o := range options {
		o(sysInfo)
	}

	ops := []infoCollector{
		applyNetworkingInfo,
		applyAppArmorInfo,
		applySeccompInfo,
		applyCgroupNsInfo,
	}

	m, err := cgroupsV2.Load(sysInfo.cg2GroupPath)
	if err != nil {
		log.G(context.TODO()).Warn(err)
	} else {
		sysInfo.cg2Controllers = make(map[string]struct{})
		controllers, err := m.Controllers()
		if err != nil {
			log.G(context.TODO()).Warn(err)
		}
		for _, c := range controllers {
			sysInfo.cg2Controllers[c] = struct{}{}
		}
		ops = append(ops,
			applyMemoryCgroupInfoV2,
			applyCPUCgroupInfoV2,
			applyIOCgroupInfoV2,
			applyCPUSetCgroupInfoV2,
			applyPIDSCgroupInfoV2,
			applyDevicesCgroupInfoV2,
		)
	}

	for _, o := range ops {
		o(sysInfo)
	}
	return sysInfo
}

func getSwapLimitV2() bool {
	_, g, err := cgroups.ParseCgroupFileUnified("/proc/self/cgroup")
	if err != nil {
		return false
	}

	if g == "" {
		return false
	}

	cGroupPath := path.Join("/sys/fs/cgroup", g, "memory.swap.max")
	if _, err = os.Stat(cGroupPath); os.IsNotExist(err) {
		return false
	}
	return true
}

func applyMemoryCgroupInfoV2(info *SysInfo) {
	if _, ok := info.cg2Controllers["memory"]; !ok {
		info.Warnings = append(info.Warnings, "Unable to find memory controller")
		return
	}

	info.MemoryLimit = true
	info.SwapLimit = getSwapLimitV2()
	info.MemoryReservation = true
	info.OomKillDisable = false
	info.MemorySwappiness = false
	info.KernelMemory = false
	info.KernelMemoryTCP = false
}

func applyCPUCgroupInfoV2(info *SysInfo) {
	if _, ok := info.cg2Controllers["cpu"]; !ok {
		info.Warnings = append(info.Warnings, "Unable to find cpu controller")
		return
	}
	info.CPUShares = true
	info.CPUCfs = true
	info.CPURealtime = false
}

func applyIOCgroupInfoV2(info *SysInfo) {
	if _, ok := info.cg2Controllers["io"]; !ok {
		info.Warnings = append(info.Warnings, "Unable to find io controller")
		return
	}

	info.BlkioWeight = true
	info.BlkioWeightDevice = true
	info.BlkioReadBpsDevice = true
	info.BlkioWriteBpsDevice = true
	info.BlkioReadIOpsDevice = true
	info.BlkioWriteIOpsDevice = true
}

func applyCPUSetCgroupInfoV2(info *SysInfo) {
	if _, ok := info.cg2Controllers["cpuset"]; !ok {
		info.Warnings = append(info.Warnings, "Unable to find cpuset controller")
		return
	}
	info.Cpuset = true

	cpus, err := os.ReadFile(path.Join("/sys/fs/cgroup", info.cg2GroupPath, "cpuset.cpus.effective"))
	if err != nil {
		return
	}
	info.Cpus = strings.TrimSpace(string(cpus))

	mems, err := os.ReadFile(path.Join("/sys/fs/cgroup", info.cg2GroupPath, "cpuset.mems.effective"))
	if err != nil {
		return
	}
	info.Mems = strings.TrimSpace(string(mems))
}

func applyPIDSCgroupInfoV2(info *SysInfo) {
	if _, ok := info.cg2Controllers["pids"]; !ok {
		info.Warnings = append(info.Warnings, "Unable to find pids controller")
		return
	}
	info.PidsLimit = true
}

func applyDevicesCgroupInfoV2(info *SysInfo) {
	info.CgroupDevicesEnabled = !userns.RunningInUserNS()
}
