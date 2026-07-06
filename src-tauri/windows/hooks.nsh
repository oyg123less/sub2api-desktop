; NSIS hooks: stop the sidecar process so its exe can be overwritten.
!macro NSIS_HOOK_PREINSTALL
  nsExec::Exec 'taskkill /F /IM "sub2api-sidecar.exe"'
  Sleep 500
!macroend

!macro NSIS_HOOK_PREUNINSTALL
  nsExec::Exec 'taskkill /F /IM "sub2api-sidecar.exe"'
  Sleep 500
!macroend
