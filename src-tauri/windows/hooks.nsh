!include LogicLib.nsh

!define AMBER_SHUTDOWN_ATTEMPTS 40
!define AMBER_SHUTDOWN_INTERVAL_MS 250

; Wait for a process to disappear instead of assuming a fixed sleep is enough.
!macro AMBER_WAIT_PROCESS_EXIT PROCESS_NAME
  StrCpy $R8 0
  ${Do}
    nsExec::Exec 'cmd /C tasklist /FI "IMAGENAME eq ${PROCESS_NAME}" /NH | find /I "${PROCESS_NAME}" >NUL'
    Pop $R9
    ${If} $R9 != 0
      ${Break}
    ${EndIf}
    Sleep ${AMBER_SHUTDOWN_INTERVAL_MS}
    IntOp $R8 $R8 + 1
  ${LoopUntil} $R8 >= ${AMBER_SHUTDOWN_ATTEMPTS}

  nsExec::Exec 'cmd /C tasklist /FI "IMAGENAME eq ${PROCESS_NAME}" /NH | find /I "${PROCESS_NAME}" >NUL'
  Pop $R9
  ${If} $R9 = 0
    MessageBox MB_ICONSTOP|MB_OK "Amber could not stop ${PROCESS_NAME}. Close Amber and retry the installer."
    Abort
  ${EndIf}
!macroend

; Process exit and executable-handle release are separate events on Windows.
!macro AMBER_WAIT_SIDECAR_WRITABLE
  StrCpy $R8 0
  ${Do}
    ${IfNot} ${FileExists} "$INSTDIR\sub2api-sidecar.exe"
      ${Break}
    ${EndIf}
    ClearErrors
    FileOpen $R9 "$INSTDIR\sub2api-sidecar.exe" a
    ${IfNot} ${Errors}
      FileClose $R9
      ${Break}
    ${EndIf}
    Sleep ${AMBER_SHUTDOWN_INTERVAL_MS}
    IntOp $R8 $R8 + 1
  ${LoopUntil} $R8 >= ${AMBER_SHUTDOWN_ATTEMPTS}

  ${If} ${FileExists} "$INSTDIR\sub2api-sidecar.exe"
    ClearErrors
    FileOpen $R9 "$INSTDIR\sub2api-sidecar.exe" a
    ${If} ${Errors}
      MessageBox MB_ICONSTOP|MB_OK "Amber is still releasing sub2api-sidecar.exe. Close Amber and retry the installer."
      Abort
    ${Else}
      FileClose $R9
    ${EndIf}
  ${EndIf}
!macroend

; Stop the desktop supervisor first so it cannot respawn the sidecar, then
; verify both process termination and release of the sidecar executable lock.
!macro AMBER_STOP_PROCESS_TREE
  nsExec::Exec 'taskkill /F /T /IM "sub2api-desktop.exe"'
  Pop $R9
  !insertmacro AMBER_WAIT_PROCESS_EXIT "sub2api-desktop.exe"
  nsExec::Exec 'taskkill /F /IM "sub2api-sidecar.exe"'
  Pop $R9
  !insertmacro AMBER_WAIT_PROCESS_EXIT "sub2api-sidecar.exe"
  !insertmacro AMBER_WAIT_SIDECAR_WRITABLE
!macroend

!macro NSIS_HOOK_PREINSTALL
  !insertmacro AMBER_STOP_PROCESS_TREE
!macroend

!macro NSIS_HOOK_PREUNINSTALL
  !insertmacro AMBER_STOP_PROCESS_TREE
!macroend
