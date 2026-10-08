Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "WinVer.nsh"
!include "x64.nsh"

Name "ZPR Machine Setup (Development)"
OutFile "${OUTPUT}"
InstallDir "$LOCALAPPDATA\ZPR\EnrollmentSetup"
RequestExecutionLevel user
SetCompressor /SOLID lzma
VIProductVersion "${VERSION}"
VIAddVersionKey "ProductName" "ZPR Machine Setup (Development)"
VIAddVersionKey "FileDescription" "Per-user enrollment wizard only; no network adapter"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "LegalCopyright" "ZPR Development Team"
!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "WINDOWS-README.txt"
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Function .onInit
    SetShellVarContext current
    ${IfNot} ${IsNativeAMD64}
        MessageBox MB_ICONSTOP "This development installer requires Windows 11 x64." /SD IDOK
        Abort
    ${EndIf}
    SetRegView 64
    ReadRegStr $0 HKLM "SOFTWARE\Microsoft\Windows NT\CurrentVersion" "CurrentBuildNumber"
    ReadRegStr $1 HKLM "SOFTWARE\Microsoft\Windows NT\CurrentVersion" "InstallationType"
    ${If} $0 < 22000
    ${OrIf} $1 != "Client"
        MessageBox MB_ICONSTOP "This development installer targets Windows 11 desktop, not Windows 10 or Server." /SD IDOK
        Abort
    ${EndIf}
FunctionEnd

Section "Enrollment wizard"
    SetShellVarContext current
    SetOutPath "$INSTDIR"
    File /oname=zpr-enrollment-setup.exe "${PAYLOAD}"
    File /oname=setup.example.json "setup.example.json"
    File /oname=README.txt "WINDOWS-README.txt"
    File "launch-setup.cmd"
    WriteUninstaller "$INSTDIR\Uninstall.exe"
    CreateDirectory "$SMPROGRAMS\ZPR Machine Setup"
    CreateShortcut "$SMPROGRAMS\ZPR Machine Setup\ZPR Machine Setup (Development).lnk" "$INSTDIR\launch-setup.cmd"
    CreateShortcut "$SMPROGRAMS\ZPR Machine Setup\Setup instructions.lnk" "$INSTDIR\README.txt"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPREnrollmentSetup" "DisplayName" "ZPR Machine Setup (Development, per-user)"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPREnrollmentSetup" "DisplayVersion" "${VERSION}"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPREnrollmentSetup" "UninstallString" '"$INSTDIR\Uninstall.exe"'
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPREnrollmentSetup" "InstallLocation" "$INSTDIR"
    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPREnrollmentSetup" "NoModify" 1
    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPREnrollmentSetup" "NoRepair" 1
SectionEnd

Section "Uninstall"
    SetShellVarContext current
    SetRegView 64
    Delete "$INSTDIR\zpr-enrollment-setup.exe"
    Delete "$INSTDIR\setup.example.json"
    Delete "$INSTDIR\README.txt"
    Delete "$INSTDIR\launch-setup.cmd"
    Delete "$INSTDIR\Uninstall.exe"
    Delete "$SMPROGRAMS\ZPR Machine Setup\ZPR Machine Setup (Development).lnk"
    Delete "$SMPROGRAMS\ZPR Machine Setup\Setup instructions.lnk"
    RMDir "$SMPROGRAMS\ZPR Machine Setup"
    RMDir "$INSTDIR"
    DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPREnrollmentSetup"
    MessageBox MB_ICONINFORMATION "Enrollment keys and your setup.json/CA configuration were preserved. Removing this wizard does not revoke enrollment or remove an adapter." /SD IDOK
SectionEnd
