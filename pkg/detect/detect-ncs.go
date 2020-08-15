package detect

// ncsStatusToText does what should be done in the go-ncs package
func ncsStatusToText(s int) string {
	statuses := map[int]string{
		// StatusOK when the device is OK.
		0: "StatusOK",
		// StatusBusy means device is busy, retry later.
		-1: "StatusBusy",
		// StatusError communicating with the device.
		-2: "StatusError",
		// StatusOutOfMemory means device out of memory.
		-3: "StatusOutOfMemory",
		// StatusDeviceNotFound means no device at the given index or name.
		-4: "StatusDeviceNotFound",
		// StatusInvalidParameters when at least one of the given parameters is wrong.
		-5: "StatusInvalidParameters",
		// StatusTimeout in the communication with the device.
		-6: "StatusTimeout",
		// StatusCmdNotFound means the file to boot Myriad was not found.
		-7: "StatusCmdNotFound",
		// StatusNoData means no data to return, call LoadTensor first.
		-8: "StatusNoData",
		// StatusGone means the graph or device has been closed during the operation.
		-9: "StatusGone",
		// StatusUnsupportedGraphFile means the graph file version is not supported.
		-10: "StatusUnsupportedGraphFile",
		// StatusMyriadError when an error has been reported by the device, use MVNC_DEBUG_INFO.
		-11: "StatusMyriadError",
	}
	if v, ok := statuses[s]; ok {
		return v
	}
	return "StatusUnknown"
}
