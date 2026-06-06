; Sirsi Pantheon — Windows Installer (NSIS)
; Builds SirsiPantheon-VERSION-windows-setup.exe
;
; Usage: makensis /DVERSION=0.19.0 windows-installer.nsi
; Requires: NSIS 3.x (https://nsis.sourceforge.io)

!include "MUI2.nsh"

; --- Metadata ---
Name "Sirsi Pantheon ${VERSION}"
OutFile "..\bin\SirsiPantheon-${VERSION}-windows-setup.exe"
InstallDir "$LOCALAPPDATA\Sirsi\Pantheon"
InstallDirRegKey HKCU "Software\Sirsi\Pantheon" "InstallDir"
RequestExecutionLevel user

; --- UI ---
!define MUI_ABORTWARNING
!define MUI_WELCOMEPAGE_TITLE "Sirsi Pantheon ${VERSION}"
!define MUI_WELCOMEPAGE_TEXT "Find and fix infrastructure waste on your machine.$\r$\n$\r$\n81 rules. Zero config. Zero telemetry.$\r$\n$\r$\nThis installer will add the sirsi CLI to your PATH."

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_LANGUAGE "English"

; --- Install ---
Section "Sirsi Pantheon" SecMain
    SetOutPath "$INSTDIR"

    ; Copy all binaries
    File "..\bin\windows\sirsi.exe"
    File "..\bin\windows\sirsi-agent.exe"

    ; Copy configs
    SetOutPath "$INSTDIR\configs"
    File "..\configs\default_rules.yaml"

    ; Copy license
    SetOutPath "$INSTDIR"
    File "..\LICENSE"

    ; Write uninstaller
    WriteUninstaller "$INSTDIR\uninstall.exe"

    ; Registry keys for Add/Remove Programs
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SirsiPantheon" \
        "DisplayName" "Sirsi Pantheon ${VERSION}"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SirsiPantheon" \
        "UninstallString" "$INSTDIR\uninstall.exe"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SirsiPantheon" \
        "DisplayVersion" "${VERSION}"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SirsiPantheon" \
        "Publisher" "Sirsi Technologies"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SirsiPantheon" \
        "URLInfoAbout" "https://sirsi.ai/pantheon"

    ; Add to user PATH via registry (no plugin needed)
    ReadRegStr $0 HKCU "Environment" "Path"
    StrCmp $0 "" 0 +2
        StrCpy $0 ""
    ; Check if already in PATH
    StrCpy $1 $0
    Push $1
    Push "$INSTDIR"
    Call StrContains
    Pop $2
    StrCmp $2 "" 0 PathDone
    ; Append to PATH
    StrCmp $0 "" 0 +2
        StrCpy $0 "$INSTDIR"
    StrCmp $0 "$INSTDIR" PathWrite
        StrCpy $0 "$0;$INSTDIR"
    PathWrite:
    WriteRegExpandStr HKCU "Environment" "Path" "$0"
    ; Notify Windows of environment change
    SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=5000
    PathDone:

    ; Store install dir
    WriteRegStr HKCU "Software\Sirsi\Pantheon" "InstallDir" "$INSTDIR"
SectionEnd

; --- Uninstall ---
Section "Uninstall"
    ; Remove from user PATH
    ReadRegStr $0 HKCU "Environment" "Path"
    ; Simple removal: replace ;$INSTDIR with empty, and $INSTDIR; with empty
    Push $0
    Push ";$INSTDIR"
    Push ""
    Call un.StrReplace
    Pop $0
    Push $0
    Push "$INSTDIR;"
    Push ""
    Call un.StrReplace
    Pop $0
    ; Handle case where it's the only entry
    StrCmp $0 "$INSTDIR" 0 +2
        StrCpy $0 ""
    WriteRegExpandStr HKCU "Environment" "Path" "$0"
    SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=5000

    ; Remove files
    Delete "$INSTDIR\sirsi.exe"
    Delete "$INSTDIR\sirsi-agent.exe"
    ; Legacy: clean up standalone deity binaries from pre-2026-06-06 installs
    ; (removed as redundant with `sirsi` subcommands). Delete is a no-op if absent.
    Delete "$INSTDIR\sirsi-anubis.exe"
    Delete "$INSTDIR\sirsi-guard.exe"
    Delete "$INSTDIR\sirsi-maat.exe"
    Delete "$INSTDIR\sirsi-scarab.exe"
    Delete "$INSTDIR\sirsi-thoth.exe"
    Delete "$INSTDIR\configs\default_rules.yaml"
    Delete "$INSTDIR\LICENSE"
    Delete "$INSTDIR\uninstall.exe"
    RMDir "$INSTDIR\configs"
    RMDir "$INSTDIR"

    ; Remove registry keys
    DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SirsiPantheon"
    DeleteRegKey HKCU "Software\Sirsi\Pantheon"
SectionEnd

; --- String helpers (no plugins) ---

Function StrContains
    ; Check if $1 contains $0
    Exch $R1 ; search string
    Exch
    Exch $R2 ; haystack
    Push $R3
    Push $R4
    StrLen $R3 $R1
    StrCpy $R4 0
    loop:
        StrCpy $0 $R2 $R3 $R4
        StrCmp $0 "" notfound
        StrCmp $0 $R1 found
        IntOp $R4 $R4 + 1
        Goto loop
    found:
        StrCpy $0 $R1
        Goto done
    notfound:
        StrCpy $0 ""
    done:
    Pop $R4
    Pop $R3
    Pop $R2
    Pop $R1
    Push $0
FunctionEnd

Function un.StrReplace
    Exch $R2 ; replacement
    Exch
    Exch $R1 ; search
    Exch 2
    Exch $R0 ; string
    Push $R3
    Push $R4
    Push $R5
    StrLen $R3 $R1
    StrCpy $R4 0
    StrCpy $R5 ""
    loop:
        StrCpy $0 $R0 $R3 $R4
        StrCmp $0 "" done
        StrCmp $0 $R1 found
        StrCpy $0 $R0 1 $R4
        StrCpy $R5 "$R5$0"
        IntOp $R4 $R4 + 1
        Goto loop
    found:
        StrCpy $R5 "$R5$R2"
        IntOp $R4 $R4 + $R3
        Goto loop
    done:
    StrCpy $R0 $R5
    Pop $R5
    Pop $R4
    Pop $R3
    Pop $R2
    Pop $R1
    Exch $R0
FunctionEnd
