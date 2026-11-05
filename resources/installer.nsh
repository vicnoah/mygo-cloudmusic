!macro customInit
  ; Check if MyGo 云音乐 is running before starting installation
  nsExec::ExecToStack 'cmd.exe /c "tasklist /NH /FI "IMAGENAME eq MyGo 云音乐.exe" | findstr /I "MyGo""'
  Pop $0
  Pop $1
  StrCmp $0 "0" 0 not_running
    IfSilent do_kill
    MessageBox MB_OKCANCEL|MB_ICONQUESTION "检测到 MyGo 云音乐 正在运行。$\r$\n$\r$\n点击“确定”将自动关闭程序并继续覆盖安装，点击“取消”退出。" IDOK do_kill
    Abort
  do_kill:
    nsExec::Exec 'taskkill /F /T /IM "MyGo 云音乐.exe"'
    Sleep 500
  not_running:
!macroend

!macro customInstall
  ; Configure Windows Defender Firewall rule to allow inbound DLNA audio streaming
  nsExec::Exec 'netsh advfirewall firewall delete rule name="MyGo 云音乐"'
  nsExec::Exec 'netsh advfirewall firewall add rule name="MyGo 云音乐" dir=in action=allow program="$INSTDIR\MyGo 云音乐.exe" enable=yes profile=private,public'
!macroend

!macro customUnInstall
  nsExec::Exec 'netsh advfirewall firewall delete rule name="MyGo 云音乐"'
  nsExec::Exec 'taskkill /F /T /IM "MyGo 云音乐.exe"'
  Sleep 500
!macroend
