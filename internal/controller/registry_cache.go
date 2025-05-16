/*
Copyright (c) 2025 SUSE LLC

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

package controller

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/adhocore/gronx"
	"k8s.io/apimachinery/pkg/types"

	"github.com/rancher/sbombastic/api/v1alpha1"
)

// RegistryInfo is for entry in RegistryCache.
type RegistryInfo struct {
	name                       string
	namespace                  string
	cronExpr                   string
	nextSchedule               time.Time
	suspended                  bool
	failedJobsHistoryLimit     uint8
	successfulJobsHistoryLimit uint8
}

// RegistryCache contains cache for all registries
// Access of field 'registries' must go thru type methods.
type RegistryCache struct {
	mutex      sync.RWMutex
	registries map[types.NamespacedName]*RegistryInfo
}

// For all cached registries.
var regCache = RegistryCache{
	mutex:      sync.RWMutex{},
	registries: make(map[types.NamespacedName]*RegistryInfo),
}

// CalcNextSchedule calculates the next scheduling timestamp by registry's cron setting.
func (rc *RegistryCache) calcNextSchedule(regNsName types.NamespacedName) {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	if regInfo, ok := rc.registries[regNsName]; ok {
		if nextSchedule, err := gronx.NextTick(regInfo.cronExpr, false); err == nil {
			regInfo.nextSchedule = nextSchedule
		}
	}
}

// Update or add a RegistryInfo entry in regCache. The registry's cron string is composed before updating regCache.
func (rc *RegistryCache) update(regNsName types.NamespacedName, spec v1alpha1.RegistrySpec) {
	const numFields int = 5
	cron := spec.DiscoveryJob.Cron
	var segmemts [numFields]string
	var cronExpr string
	var defaultCronHour int8
	var emptyCron v1alpha1.NumericCron

	disableSchedule := true
	if spec.DiscoveryJob.Cron == emptyCron {
		cron.Hour = &defaultCronHour
	}
	cronFields := [numFields]*int8{nil, cron.Hour, cron.DayOfMonth, cron.Month, cron.DayOfWeek}
	for i := range numFields {
		if cronFields[i] == nil {
			segmemts[i] = "*"
		} else {
			segmemts[i] = strconv.Itoa(int(*cronFields[i]))
			disableSchedule = false
		}
	}
	if !disableSchedule {
		cronExpr = strings.Join(segmemts[:], " ")
	}

	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	regInfo, ok := rc.registries[regNsName]
	if ok {
		specRegInfo := RegistryInfo{
			name:                       regNsName.Name,
			namespace:                  regNsName.Namespace,
			cronExpr:                   cronExpr,
			nextSchedule:               regInfo.nextSchedule,
			suspended:                  spec.DiscoveryJob.Suspend,
			failedJobsHistoryLimit:     spec.DiscoveryJob.FailedJobsHistoryLimit,
			successfulJobsHistoryLimit: spec.DiscoveryJob.SuccessfulJobsHistoryLimit,
		}
		if *regInfo == specRegInfo {
			return
		}
	} else {
		regInfo = &RegistryInfo{
			name:      regNsName.Name,
			namespace: regNsName.Namespace,
		}
	}
	regInfo.cronExpr = cronExpr
	regInfo.suspended = spec.DiscoveryJob.Suspend
	regInfo.nextSchedule = time.Time{}
	regInfo.failedJobsHistoryLimit = spec.DiscoveryJob.FailedJobsHistoryLimit
	regInfo.successfulJobsHistoryLimit = spec.DiscoveryJob.SuccessfulJobsHistoryLimit
	if cronExpr != "" {
		if nextSchedule, err := gronx.NextTick(regInfo.cronExpr, false); err == nil {
			regInfo.nextSchedule = nextSchedule
		}
	}
	rc.registries[regNsName] = regInfo
}

// Delete a RegistryInfo entry in regCache
func (rc *RegistryCache) delete(regNsName types.NamespacedName) (RegistryInfo, bool) {
	var found bool
	var info RegistryInfo

	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	if v, ok := rc.registries[regNsName]; ok {
		found = true
		info = *v
	}
	delete(rc.registries, regNsName)

	return info, found
}

// Purge deletes all entries in regCache
func (rc *RegistryCache) purge() {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	for regNsName := range rc.registries {
		delete(rc.registries, regNsName)
	}
}

// Copy returns a copy of regCache.
func (rc *RegistryCache) copy() map[types.NamespacedName]RegistryInfo {
	ret := make(map[types.NamespacedName]RegistryInfo, len(rc.registries))

	rc.mutex.RLock()
	defer rc.mutex.RUnlock()

	for k, v := range rc.registries {
		ret[k] = *v
	}

	return ret
}
