!macro customInit
  ; Check if 肥牛音乐 is running before starting installation
  nsExec::ExecToStack 'cmd.exe /c "tasklist /NH /FI "IMAGENAME eq Wagyu Music.exe" | findstr /I "Wagyu Music""'
  Pop $0
  Pop $1
  StrCmp $0 "0" 0 not_running
    IfSilent do_kill
    MessageBox MB_OKCANCEL|MB_ICONQUESTION "检测到肥牛音乐正在运行。$\r$\n$\r$\n点击“确定”将自动关闭程序并继续覆盖安装，点击“取消”退出。" IDOK do_kill
    Abort
  do_kill:
    nsExec::Exec 'taskkill /F /T /IM "Wagyu Music.exe"'
    Sleep 500
  not_running:
!macroend

!macro customInstall
  ; A version under an old name installed into the same folder: remove its
  ; executable, shortcuts and firewall rule, then let the local network
  ; reach DLNA through this one.
  Delete "$INSTDIR\MyGo 云音乐.exe"
  Delete "$SMPROGRAMS\MyGo 云音乐.lnk"
  Delete "$DESKTOP\MyGo 云音乐.lnk"
  nsExec::Exec 'netsh advfirewall firewall delete rule name="MyGo 云音乐"'
  nsExec::Exec 'netsh advfirewall firewall delete rule name="肥牛音乐"'
  nsExec::Exec 'netsh advfirewall firewall add rule name="肥牛音乐" dir=in action=allow program="$INSTDIR\Wagyu Music.exe" enable=yes profile=private,public'
!macroend

!macro customUnInstall
  nsExec::Exec 'netsh advfirewall firewall delete rule name="肥牛音乐"'
  nsExec::Exec 'taskkill /F /T /IM "Wagyu Music.exe"'
  Sleep 500
!macroend
