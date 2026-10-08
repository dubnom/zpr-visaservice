on run argv
    set bundlePath to item 1 of argv
    set configPath to item 2 of argv
    set configured to do shell script "if test -f " & quoted form of configPath & "; then printf yes; else printf no; fi"
    if configured is not "yes" then
        display dialog "An administrator must provision trusted enrollment configuration first. No enrollment request was sent." & return & return & configPath buttons {"Open instructions", "Cancel"} default button "Open instructions"
        if button returned of result is "Open instructions" then
            do shell script "/usr/bin/open " & quoted form of (bundlePath & "/Resources/README.txt")
        end if
        return
    end if
    set commandText to quoted form of (bundlePath & "/MacOS/zpr-enrollment-setup") & " -config " & quoted form of configPath & " -user-state -require-key-protection macos-keychain -open-browser"
    tell application "Terminal"
        activate
        do script commandText
    end tell
end run
