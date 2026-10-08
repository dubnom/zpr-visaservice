package enrollment

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func currentWindowsSID() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

func windowsPath(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path ||
		strings.HasPrefix(path, `\\`) || strings.Contains(path[2:], ":") {
		return errors.New("setup requires a clean absolute local drive path, not UNC, device paths or alternate streams")
	}
	return nil
}

func checkWindowsACL(handle windows.Handle, private bool) error {
	sid, err := currentWindowsSID()
	if err != nil {
		return err
	}
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil {
		return errors.New("setup file has no verifiable owner")
	}
	trusted := func(value string) bool {
		return value == sid || value == "S-1-5-18" || (!private && value == "S-1-5-32-544")
	}
	if !trusted(owner.String()) {
		return errors.New("setup file is not owned by the current user or a trusted system administrator")
	}
	acl, _, err := descriptor.DACL()
	if err != nil || acl == nil || acl.AceCount == 0 {
		return errors.New("setup file requires an explicit nonempty access-control list")
	}
	const writable = windows.GENERIC_ALL | windows.GENERIC_WRITE | windows.WRITE_DAC |
		windows.WRITE_OWNER | windows.DELETE | windows.FILE_WRITE_DATA |
		windows.FILE_APPEND_DATA | windows.FILE_WRITE_EA | windows.FILE_WRITE_ATTRIBUTES |
		0x40 // FILE_DELETE_CHILD
	for index := uint32(0); index < uint32(acl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, index, &ace); err != nil {
			return err
		}
		if !private && ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		switch ace.Header.AceType {
		case windows.ACCESS_DENIED_ACE_TYPE:
			continue
		case windows.ACCESS_ALLOWED_ACE_TYPE:
			aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
			if !trusted(aceSID) && (private || uint32(ace.Mask)&writable != 0) {
				return errors.New("setup file grants unsafe access to another principal")
			}
		default:
			return errors.New("setup file has an unsupported access-control entry")
		}
	}
	return nil
}

func openWindowsFile(path string, directory, private bool) (*os.File, error) {
	if err := windowsPath(path); err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	access, share := uint32(windows.GENERIC_READ|windows.READ_CONTROL), uint32(windows.FILE_SHARE_READ)
	if directory {
		access = windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL
		share |= windows.FILE_SHARE_WRITE
	}
	handle, err := windows.CreateFile(name, access, share, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	file := os.NewFile(uintptr(handle), path)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		file.Close()
		return nil, err
	}
	isDirectory := info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || isDirectory != directory ||
		(!directory && info.NumberOfLinks != 1) {
		file.Close()
		return nil, errors.New("setup path must not be a reparse point, wrong file type or multiply-linked file")
	}
	if err := checkWindowsACL(handle, private); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

func createWindowsPrivateDirectory(path string) error {
	if err := windowsPath(path); err != nil {
		return err
	}
	sid, err := currentWindowsSID()
	if err != nil {
		return err
	}
	descriptor, err := windows.SecurityDescriptorFromString("O:" + sid + "D:P(A;OICI;FA;;;" + sid + ")(A;OICI;FA;;;SY)")
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attributes := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: descriptor}
	if err := windows.CreateDirectory(name, &attributes); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return &os.PathError{Op: "mkdir", Path: path, Err: err}
	}
	file, err := openWindowsFile(path, true, true)
	if err != nil {
		return err
	}
	return file.Close()
}

func readSetupFile(path string) ([]byte, error) {
	if err := checkWindowsParents(path); err != nil {
		return nil, err
	}
	file, err := openWindowsFile(path, false, false)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 65537))
	if err != nil {
		return nil, err
	}
	if len(data) > 65536 {
		return nil, errors.New("setup configuration or CA file exceeds 65536 bytes")
	}
	return data, nil
}

func checkWindowsParents(path string) error {
	if err := windowsPath(path); err != nil {
		return err
	}
	for parent := filepath.Dir(path); parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
		file, err := openWindowsFile(parent, true, false)
		if err != nil {
			return fmt.Errorf("unsafe setup parent %s: %w", parent, err)
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	return nil
}

func LoadUserSetupConfig(path string) (SetupConfig, error) {
	if windows.GetCurrentProcessToken().IsElevated() {
		return SetupConfig{}, errors.New("desktop setup must run as the logged-in user, not elevated")
	}
	base, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return SetupConfig{}, err
	}
	parent := filepath.Join(base, "ZPR")
	state := filepath.Join(parent, "EnrollmentDevelopment")
	config, err := loadSetupConfig(path, state)
	if err != nil {
		return SetupConfig{}, err
	}
	if err := checkWindowsParents(parent); err != nil {
		return SetupConfig{}, err
	}
	if err := os.Mkdir(parent, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return SetupConfig{}, err
	}
	file, err := openWindowsFile(parent, true, false)
	if err != nil {
		return SetupConfig{}, err
	}
	if err := file.Close(); err != nil {
		return SetupConfig{}, err
	}
	return config, nil
}
