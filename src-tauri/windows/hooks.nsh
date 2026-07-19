; Stop the desktop supervisor before the sidecar so it cannot respawn and
; reacquire the executable lock during an in-place upgrade.
!macro AMBER_STOP_PROCESS_TREE
  nsExec::Exec 'taskkill /F /T /IM "sub2api-desktop.exe"'
  Sleep 1200
  nsExec::Exec 'taskkill /F /IM "sub2api-sidecar.exe"'
  Sleep 500
!macroend

!macro NSIS_HOOK_PREINSTALL
  !insertmacro AMBER_STOP_PROCESS_TREE
!macroend

!macro NSIS_HOOK_PREUNINSTALL
  !insertmacro AMBER_STOP_PROCESS_TREE
!macroend
