Unicode True
!include "MUI2.nsh"

!ifndef PRODUCT_VERSION
  !define PRODUCT_VERSION "0.1.0"
!endif
!ifndef BIN_DIR
  !define BIN_DIR "dist"
!endif
!ifndef OUT_FILE
  !define OUT_FILE "SingTail-windows-amd64-setup.exe"
!endif

!define PRODUCT_NAME "SingTail"
!define SERVICE_NAME "SingTailService"
!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_NAME}"

Name "${PRODUCT_NAME} ${PRODUCT_VERSION}"
OutFile "${OUT_FILE}"
InstallDir "$PROGRAMFILES64\${PRODUCT_NAME}"
InstallDirRegKey HKLM "${UNINSTALL_KEY}" "InstallLocation"
RequestExecutionLevel admin

!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\singtail.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Launch ${PRODUCT_NAME}"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

Section "${PRODUCT_NAME}" SecMain
  SetOutPath "$INSTDIR"

  ; Stop running desktop process and background service before replacing binaries
  nsExec::ExecToLog 'taskkill /F /IM singtail.exe'
  nsExec::ExecToLog 'sc stop "${SERVICE_NAME}"'
  Sleep 1200
  nsExec::ExecToLog 'taskkill /F /IM singtail-service.exe'

  File "${BIN_DIR}\singtail.exe"
  File "${BIN_DIR}\singtail-service.exe"
  File "${BIN_DIR}\README.md"
  File "install-service.bat"
  File "uninstall-service.bat"

  ; Register and start Windows background daemon service
  nsExec::ExecToLog '"$INSTDIR\singtail-service.exe" -install'
  nsExec::ExecToLog 'sc start "${SERVICE_NAME}"'

  ; Start Menu and Desktop shortcuts
  CreateDirectory "$SMPROGRAMS\${PRODUCT_NAME}"
  CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}\${PRODUCT_NAME}.lnk" "$INSTDIR\singtail.exe"
  CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}\Uninstall ${PRODUCT_NAME}.lnk" "$INSTDIR\Uninstall.exe"
  CreateShortcut "$DESKTOP\${PRODUCT_NAME}.lnk" "$INSTDIR\singtail.exe"

  ; Uninstaller and Add/Remove Programs registration
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKLM "${UNINSTALL_KEY}" "DisplayName" "${PRODUCT_NAME}"
  WriteRegStr HKLM "${UNINSTALL_KEY}" "DisplayVersion" "${PRODUCT_VERSION}"
  WriteRegStr HKLM "${UNINSTALL_KEY}" "Publisher" "${PRODUCT_NAME}"
  WriteRegStr HKLM "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "${UNINSTALL_KEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegDWORD HKLM "${UNINSTALL_KEY}" "NoModify" 1
  WriteRegDWORD HKLM "${UNINSTALL_KEY}" "NoRepair" 1
SectionEnd

Section "Uninstall"
  nsExec::ExecToLog 'taskkill /F /IM singtail.exe'
  nsExec::ExecToLog 'sc stop "${SERVICE_NAME}"'
  Sleep 1500
  nsExec::ExecToLog 'taskkill /F /IM singtail-service.exe'
  nsExec::ExecToLog '"$INSTDIR\singtail-service.exe" -uninstall'

  Delete "$DESKTOP\${PRODUCT_NAME}.lnk"
  Delete "$SMPROGRAMS\${PRODUCT_NAME}\${PRODUCT_NAME}.lnk"
  Delete "$SMPROGRAMS\${PRODUCT_NAME}\Uninstall ${PRODUCT_NAME}.lnk"
  RMDir "$SMPROGRAMS\${PRODUCT_NAME}"

  Delete "$INSTDIR\singtail.exe"
  Delete "$INSTDIR\singtail-service.exe"
  Delete "$INSTDIR\README.md"
  Delete "$INSTDIR\install-service.bat"
  Delete "$INSTDIR\uninstall-service.bat"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"

  DeleteRegKey HKLM "${UNINSTALL_KEY}"
SectionEnd
