//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func main() {
	var job windows.Handle
	job, _ = windows.CreateJobObject(nil, nil)
	if job == 0 {
		fmt.Println("CreateJobObject failed")
		return
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))

	// Start a process that spawns notepad
	cmd := exec.Command("powershell.exe", "-Command", "Start-Process notepad; Start-Sleep 30")
	cmd.Start()

	// Assign to job
	processHandle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		fmt.Printf("OpenProcess failed: %v\n", err)
		windows.CloseHandle(job)
		return
	}
	if err := windows.AssignProcessToJobObject(job, processHandle); err != nil {
		fmt.Printf("AssignProcessToJobObject failed: %v\n", err)
	} else {
		fmt.Println("Assigned to job successfully")
	}
	windows.CloseHandle(processHandle)

	fmt.Println("Waiting 5 seconds...")
	time.Sleep(5 * time.Second)

	fmt.Println("Calling TerminateJobObject...")
	windows.TerminateJobObject(job, 1)
	windows.CloseHandle(job)

	time.Sleep(2 * time.Second)
	fmt.Println("Done. Check if notepad survived.")
	runtime.KeepAlive(cmd)
}
