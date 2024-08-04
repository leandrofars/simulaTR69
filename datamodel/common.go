package datamodel

import (
	"strconv"
	"strings"
	"time"
)

type DeviceID struct {
	Manufacturer string
	OUI          string
	ProductClass string
	SerialNumber string
}

func (dm *DataModel) DeviceID() DeviceID {
	return DeviceID{
		Manufacturer: dm.firstValue(
			"DeviceID.Manufacturer",
			"Device.DeviceInfo.Manufacturer",
			"InternetGatewayDevice.DeviceInfo.Manufacturer",
		),
		OUI: dm.firstValue(
			"DeviceID.OUI",
			"Device.DeviceInfo.ManufacturerOUI",
			"InternetGatewayDevice.DeviceInfo.ManufacturerOUI",
		),
		ProductClass: dm.firstValue(
			"DeviceID.ProductClass",
			"Device.DeviceInfo.ProductClass",
			"InternetGatewayDevice.DeviceInfo.ProductClass",
		),
		SerialNumber: dm.firstValue(
			"DeviceID.SerialNumber",
			"Device.DeviceInfo.SerialNumber",
			"InternetGatewayDevice.DeviceInfo.SerialNumber",
		),
	}
}

const (
	pathSerialNumber           = "DeviceInfo.SerialNumber"
	pathSoftwareVersion        = "DeviceInfo.SoftwareVersion"
	pathConnectionRequestURL   = "ManagementServer.ConnectionRequestURL"
	pathParameterKey           = "ManagementServer.ParameterKey"
	pathPeriodicInformEnable   = "ManagementServer.PeriodicInformEnable"
	pathPeriodicInformTime     = "ManagementServer.PeriodicInformTime"
	pathPeriodicInformInterval = "ManagementServer.PeriodicInformInterval"
)

func (dm *DataModel) SetSerialNumber(val string) {
	dm.SetValue(pathSerialNumber, val)
}

func (dm *DataModel) ConnectionRequestURL() Parameter {
	return dm.GetValue(pathConnectionRequestURL)
}

func (dm *DataModel) SetConnectionRequestURL(val string) {
	dm.SetValue(pathConnectionRequestURL, val)
}

func (dm *DataModel) SetParameterKey(val string) {
	dm.SetValue(pathParameterKey, val)
}

func (dm *DataModel) PeriodicInformEnabled() bool {
	val := dm.GetValue(pathPeriodicInformEnable)
	b, _ := strconv.ParseBool(val.Value)
	return b
}

func (dm *DataModel) PeriodicInformInterval() time.Duration {
	const defaultInterval = 5 * time.Minute
	const secondsInDay = int64(24 * time.Hour / time.Second)
	val := dm.GetValue(pathPeriodicInformInterval)
	i, _ := strconv.ParseInt(val.Value, 10, 32)
	if i == 0 || i > secondsInDay {
		return defaultInterval
	}
	return time.Duration(i) * time.Second
}

func (dm *DataModel) SetPeriodicInformInterval(sec int64) {
	dm.SetValue(pathPeriodicInformInterval, strconv.FormatInt(sec, 10))
}

func (dm *DataModel) PeriodicInformTime() time.Time {
	val := dm.GetValue(pathPeriodicInformTime)
	i, _ := strconv.ParseInt(val.Value, 10, 32)
	return time.Unix(i, 0)
}

func (dm *DataModel) SetPeriodicInformTime(ts time.Time) {
	dm.SetValue(pathPeriodicInformTime, strconv.FormatInt(ts.Unix(), 10))
}

func (dm *DataModel) IsPeriodicInformParameter(name string) bool {
	if strings.HasSuffix(name, pathPeriodicInformInterval) {
		return true
	}
	if strings.HasSuffix(name, pathPeriodicInformTime) {
		return true
	}
	if strings.HasSuffix(name, pathPeriodicInformEnable) {
		return true
	}
	return false
}

func (dm *DataModel) SetFirmwareVersion(ver string) {
	dm.SetValue(pathSoftwareVersion, ver)
}
