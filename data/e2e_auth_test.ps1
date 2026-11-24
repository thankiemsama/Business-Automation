# E2E auth pentest against the live server (port 8081)
$base = 'http://127.0.0.1:8081'
$secret = 'e2e-proxy-secret-0123456789abcdef'
$log = 'D:\GOLANG\data\e2e-server.log'
$results = New-Object System.Collections.Generic.List[object]

function Call {
    param([string]$Method, [string]$Url, $Body, [string]$Ip, [string]$Token = '', [string]$ContentType = 'application/json')
    $h = @{ 'X-Nexa-Client-IP' = $Ip; 'X-Nexa-Proxy-Secret' = $secret }
    if ($Token) { $h['Authorization'] = "Bearer $Token" }
    if ($ContentType) { $h['Content-Type'] = $ContentType }
    $params = @{ Method = $Method; Uri = $Url; Headers = $h; UseBasicParsing = $true }
    if ($null -ne $Body) { $params.Body = (ConvertTo-Json $Body -Compress) }
    try {
        $r = Invoke-WebRequest @params
        $j = $r.Content | ConvertFrom-Json
        [pscustomobject]@{ Status = [int]$r.StatusCode; Headers = $r.Headers; Body = $j }
    } catch {
        $resp = $_.Exception.Response
        if ($resp) {
            $reader = [System.IO.StreamReader]::new($resp.GetResponseStream())
            $text = $reader.ReadToEnd()
            try { $j = $text | ConvertFrom-Json } catch { $j = @{ raw = $text } }
            [pscustomobject]@{ Status = [int]$resp.StatusCode; Headers = $resp.Headers; Body = $j }
        } else {
            [pscustomobject]@{ Status = -1; Headers = @{}; Body = @{ raw = $_.Exception.Message } }
        }
    }
}

function CallRaw {
    param([string]$Method, [string]$Url, [string]$Text, [string]$Ip, [string]$ContentType = 'application/json')
    $h = @{ 'X-Nexa-Client-IP' = $Ip; 'X-Nexa-Proxy-Secret' = $secret }
    if ($ContentType) { $h['Content-Type'] = $ContentType }
    try {
        $r = Invoke-WebRequest -Method $Method -Uri $Url -Headers $h -Body $Text -UseBasicParsing
        $j = $r.Content | ConvertFrom-Json
        [pscustomobject]@{ Status = [int]$r.StatusCode; Headers = $r.Headers; Body = $j }
    } catch {
        $resp = $_.Exception.Response
        if ($resp) {
            $reader = [System.IO.StreamReader]::new($resp.GetResponseStream())
            $text = $reader.ReadToEnd()
            try { $j = $text | ConvertFrom-Json } catch { $j = @{ raw = $text } }
            [pscustomobject]@{ Status = [int]$resp.StatusCode; Headers = $resp.Headers; Body = $j }
        } else { [pscustomobject]@{ Status = -1; Headers = @{}; Body = @{ raw = $_.Exception.Message } } }
    }
}

function Check($Name, $Section, $Expected, $Actual, $Note = '') {
    $pass = ($Expected -eq $Actual) -or ($Expected -is [array] -and $Expected -contains $Actual)
    $results.Add([pscustomobject]@{ Section = $Section; Test = $Name; Expected = ($Expected -join '|'); Actual = $Actual; Pass = $pass; Note = $Note })
    $mark = if ($pass) { 'PASS' } else { 'FAIL' }
    Write-Host ("[{0}] {1} :: {2} (expect {3}, got {4})" -f $mark, $Section, $Name, ($Expected -join '|'), $Actual)
}

function ExtractLastLink($Path) {
    $lines = Get-Content $Path
    $hits = $lines | Where-Object { $_ -match 'account/setup#' } | Select-Object -Last 1
    if (-not $hits) { return $null }
    $frag = $hits.Split('#')[-1]
    return [System.Uri]::UnescapeDataString($frag)
}

$mainIp = '10.1.1.1'
$lockIp = '10.1.1.2'
$longIp = '10.1.1.3'
$teamIp = '10.1.1.4'
$redIp  = '10.1.1.5'
$rlIp   = '10.2.0.1'
$vIp    = '10.1.1.8'
$hdrIp  = '10.1.1.7'

$pwd12 = 'password-123456'

# ============ A. REGISTER ============
$r = Call POST "$base/api/v1/auth/register" @{ name='Main User'; email='main@e2e.test'; password=$pwd12 } $mainIp
Check 'R1 register valid' 'Register' 201 $r.Status
$data = $r.Body.data
$leak = @('password_hash','salt','totp_secret','recovery_hashes','password') | Where-Object { $data.PSObject.Properties.Name -contains $_ }
Check 'R1b register payload has no secrets' 'Register' '' ($leak -join ',') 'public fields only'
Check 'R1c register role' 'Register' 'owner' $data.role

$r = Call POST "$base/api/v1/auth/register" @{ name='Dup'; email='MAIN@E2E.TEST'; password=$pwd12 } $mainIp
Check 'R2 duplicate email case-variant' 'Register' 409 $r.Status "msg=$($r.Body.message)"

$r = Call POST "$base/api/v1/auth/register" @{ name='X'; email='r3@e2e.test'; password='short-11ch' } $mainIp
Check 'R3 password 11 chars' 'Register' 400 $r.Status

$r = Call POST "$base/api/v1/auth/register" @{ name='X'; email='r4@e2e.test'; password=('p'*129) } $mainIp
Check 'R4 password 129 chars' 'Register' 400 $r.Status

$r = Call POST "$base/api/v1/auth/register" @{ email='r5@e2e.test'; password=$pwd12 } $mainIp
Check 'R5 empty name' 'Register' 400 $r.Status

$r = Call POST "$base/api/v1/auth/register" @{ name='X'; email='not-an-email'; password=$pwd12 } $mainIp
Check 'R6 malformed email' 'Register' 400 $r.Status

$r = CallRaw POST "$base/api/v1/auth/register" '{"name":"X","email":"A <a@e2e.test>","password":"password-123456"}' $mainIp
Check 'R7 angle-bracket email' 'Register' 400 $r.Status

$r = Call POST "$base/api/v1/auth/register" @{ name='X'; email='r8@e2e.test'; password=$pwd12; role='admin' } $mainIp
Check 'R8 role injection' 'Register' 400 $r.Status "msg=$($r.Body.message)"

$r = Call POST "$base/api/v1/auth/register" @{ name='X'; email='r9@e2e.test'; password=$pwd12; admin=$true } $mainIp
Check 'R9 admin field injection' 'Register' 400 $r.Status

$r = Call POST "$base/api/v1/auth/register" @{ name='X'; email='r10@e2e.test'; password=$pwd12; industry='unknown' } $mainIp
Check 'R10 bogus industry' 'Register' 400 $r.Status

$r = Call POST "$base/api/v1/auth/register" @{ name='X'; email='r11@e2e.test'; password=$pwd12; industry='ecommerce' } $mainIp
Check 'R11 valid industry' 'Register' 201 $r.Status "msg=$($r.Body.message)"

$r = CallRaw POST "$base/api/v1/auth/register" '[1,2,3]' $mainIp
Check 'R12 JSON array body' 'Register' 400 $r.Status

$r = CallRaw POST "$base/api/v1/auth/register" '{"name":"X","email":"r13@e2e.test","password":"password-123456"}' $mainIp 'text/plain'
Check 'R13 text/plain content-type' 'Register' 415 $r.Status

# ============ B. LOGIN ============
$r = Call POST "$base/api/v1/auth/login" @{ email='main@e2e.test'; password=$pwd12 } $mainIp
Check 'L1 login valid' 'Login' 200 $r.Status
$mainToken = $r.Body.data.access_token
Check 'L1b token_type Bearer' 'Login' 'Bearer' $r.Body.data.token_type
Check 'L1c expires_in 24h' 'Login' 86400 $r.Body.data.expires_in

$r = Call POST "$base/api/v1/auth/login" @{ email='main@e2e.test'; password='wrong-password-1' } $mainIp
Check 'L2 wrong password' 'Login' 401 $r.Status
$msgL2 = $r.Body.message

$r = Call POST "$base/api/v1/auth/login" @{ email='ghost@e2e.test'; password='password-123456' } $mainIp
Check 'L3 unknown email (enum-safe)' 'Login' 401 $r.Status
Check 'L3b identical error message' 'Login' $msgL2 $r.Body.message "anti-enumeration"

$r = Call POST "$base/api/v1/auth/login" @{ email='MAIN@E2E.TEST'; password=$pwd12 } $mainIp
Check 'L4 email case-insensitive' 'Login' 200 $r.Status

$r = Call GET "$base/api/v1/auth/login" $null $mainIp
Check 'L5 GET method not allowed' 'Login' 405 $r.Status

$r = CallRaw POST "$base/api/v1/auth/login" '{}' $mainIp
Check 'L6 empty body login' 'Login' 401 $r.Status

Call POST "$base/api/v1/auth/register" @{ name='Long'; email='long@e2e.test'; password=$pwd12 } $longIp | Out-Null
$r = Call POST "$base/api/v1/auth/login" @{ email='long@e2e.test'; password=('p'*129) } $longIp
Check 'L7 129-char password at login' 'Login' 401 $r.Status

Call POST "$base/api/v1/auth/register" @{ name='Lock'; email='lock@e2e.test'; password=$pwd12 } $lockIp | Out-Null
for ($i = 1; $i -le 5; $i++) {
    $r = Call POST "$base/api/v1/auth/login" @{ email='lock@e2e.test'; password='totally-wrong-1' } $lockIp
}
Check 'L8 5th wrong attempt -> 401' 'Login-Lockout' 401 $r.Status
$r = Call POST "$base/api/v1/auth/login" @{ email='lock@e2e.test'; password=$pwd12 } $lockIp
Check 'L8b correct pw while locked' 'Login-Lockout' 401 $r.Status "msg=$($r.Body.message)"
Check 'L8c reason account_locked' 'Login-Lockout' 'account_locked' $r.Body.reason

$r = Call POST "$base/api/v1/auth/register" @{ name='Team Owner'; email='team@e2e.test'; password=$pwd12 } $teamIp
$teamToken = $r.Body.data.access_token
$r = Call POST "$base/api/v1/team" @{ name='Agent'; email='agent@e2e.test'; password='agent-pass-12345' } $teamIp $teamToken
Check 'L9 owner creates agent' 'Login-Disabled' 201 $r.Status
$agentId = $r.Body.data.id
$r = Call PUT "$base/api/v1/team/$agentId" @{ name='Agent'; email='agent@e2e.test'; disabled=$true } $teamIp $teamToken
Check 'L9b owner disables agent' 'Login-Disabled' 200 $r.Status
$r = Call POST "$base/api/v1/auth/login" @{ email='agent@e2e.test'; password='agent-pass-12345' } $teamIp
Check 'L9c disabled agent login' 'Login-Disabled' 401 $r.Status

# ============ C. PASSWORD CHANGE ============
$r = Call POST "$base/api/v1/auth/password" @{ old_password='nope-nope-12345'; password='new-password-1234' } $mainIp $mainToken
Check 'P1 wrong old password' 'Password' 400 $r.Status
$r = Call POST "$base/api/v1/auth/password" @{ old_password='short'; password='new-password-1234' } $mainIp $mainToken
Check 'P2 11-char new password' 'Password' 400 $r.Status
$r = Call POST "$base/api/v1/auth/password" @{ old_password=$pwd12; password=('p'*129) } $mainIp $mainToken
Check 'P3 129-char new password' 'Password' 400 $r.Status
$r = Call POST "$base/api/v1/auth/password" @{ old_password=$pwd12; password='new-password-1234' } $mainIp $mainToken
Check 'P4 valid change' 'Password' 200 $r.Status
$r = Call GET "$base/api/v1/auth/me" $null $mainIp $mainToken
Check 'P5 old session revoked' 'Password' 401 $r.Status

# ============ D. LOGOUT ============
$r = Call POST "$base/api/v1/auth/login" @{ email='main@e2e.test'; password='new-password-1234' } $mainIp
$mainToken = $r.Body.data.access_token
$r = Call POST "$base/api/v1/auth/logout" $null $mainIp $mainToken
Check 'O1 logout' 'Logout' 200 $r.Status
$r = Call GET "$base/api/v1/auth/me" $null $mainIp $mainToken
Check 'O2 token dead after logout' 'Logout' 401 $r.Status

# ============ E. FORGOT / REDEEM ============
$r = Call POST "$base/api/v1/auth/register" @{ name='Redeemer'; email='redeemer@e2e.test'; password=$pwd12 } $redIp
$redLogin = Call POST "$base/api/v1/auth/login" @{ email='redeemer@e2e.test'; password=$pwd12 } $redIp
$redToken = $redLogin.Body.data.access_token
$r = Call POST "$base/api/v1/auth/forgot" @{ email='redeemer@e2e.test' } $redIp
Check 'F1 forgot known email (dev sink)' 'Forgot' 200 $r.Status
$r = Call POST "$base/api/v1/auth/forgot" @{ email='nobody@e2e.test' } $redIp
Check 'F2 forgot unknown email (enum-safe)' 'Forgot' 200 $r.Status

$r = Call POST "$base/api/v1/auth/redeem" @{ token=''; password='password-123456' } $redIp
Check 'F3 redeem empty token' 'Forgot' 400 $r.Status
$r = Call POST "$base/api/v1/auth/redeem" @{ token='no-dot-at-all'; password='password-123456' } $redIp
Check 'F4 redeem without dot' 'Forgot' 400 $r.Status
$r = Call POST "$base/api/v1/auth/redeem" @{ token='tenant.'; password='password-123456' } $redIp
Check 'F5 redeem empty secret' 'Forgot' 400 $r.Status
$r = Call POST "$base/api/v1/auth/redeem" @{ token='t.'+('b'*48); password='password-123456' } $redIp
Check 'F6 redeem random token' 'Forgot' 400 $r.Status

Start-Sleep -Seconds 2
$token = ExtractLastLink $log
Check 'F7 extract reset link from dev log' 'Forgot' $true ($null -ne $token) "token=$token"
$r = Call POST "$base/api/v1/auth/redeem" @{ token=$token; password='short' } $redIp
Check 'F8 redeem valid token + short pw' 'Forgot' 400 $r.Status
$r = Call POST "$base/api/v1/auth/redeem" @{ token=$token; password='recovered-12345' } $redIp
Check 'F9 redeem valid' 'Forgot' 200 $r.Status
$r = Call GET "$base/api/v1/auth/me" $null $redIp $redToken
Check 'F10 old sessions revoked after redeem' 'Forgot' 401 $r.Status
$r = Call POST "$base/api/v1/auth/login" @{ email='redeemer@e2e.test'; password='recovered-12345' } $redIp
Check 'F11 login with new password' 'Forgot' 200 $r.Status
$r = Call POST "$base/api/v1/auth/login" @{ email='redeemer@e2e.test'; password=$pwd12 } $redIp
Check 'F12 old password rejected' 'Forgot' 401 $r.Status
$r = Call POST "$base/api/v1/auth/redeem" @{ token=$token; password='reused-12345678' } $redIp
Check 'F13 replay redeemed token' 'Forgot' 400 $r.Status "single-use"

# ============ F. VERIFY / CONFIRM ============
$r = Call POST "$base/api/v1/auth/verify" @{ email='nobody@e2e.test' } $vIp
Check 'V1 resend verify unknown email' 'Verify' 200 $r.Status "enum-safe"
$r = Call POST "$base/api/v1/auth/confirm" @{ token='garbage' } $vIp
Check 'V3 confirm garbage token' 'Verify' 400 $r.Status

# ============ G. RATE LIMIT ============
$rl401 = 0; $rl429 = 0; $lastCode = 0
for ($i = 1; $i -le 21; $i++) {
    $r = Call POST "$base/api/v1/auth/login" @{ email='rl@e2e.test'; password='wrong-password-1' } $rlIp
    $lastCode = $r.Status
    if ($r.Status -eq 401) { $rl401++ } elseif ($r.Status -eq 429) { $rl429++ }
}
Check 'RL1 first 20 -> 401' 'RateLimit' 20 $rl401 "got=$rl401"
Check 'RL2 21st -> 429' 'RateLimit' 1 $rl429 "last=$lastCode"

# ============ H. SECURITY HEADERS / MISC ============
$r = Call GET "$base/api/v1/auth/me" $null $hdrIp
Check 'H1 X-Content-Type-Options' 'Headers' 'nosniff' $r.Headers.'X-Content-Type-Options'
Check 'H2 Cache-Control' 'Headers' 'no-store' $r.Headers.'Cache-Control'
$r = Call GET "$base/api/v1/does-not-exist" $null $hdrIp
Check 'H3 unknown endpoint 404' 'Headers' 404 $r.Status

$mfaLogin = Call POST "$base/api/v1/auth/login" @{ email='main@e2e.test'; password='new-password-1234' } $mainIp
$mfaToken = $mfaLogin.Body.data.access_token
$r = Call POST "$base/api/v1/account/mfa" @{ action='setup'; password='new-password-1234' } $mainIp $mfaToken
Check 'M1 mfa setup without security key' 'MFA' 503 $r.Status "msg=$($r.Body.message)"

# ============ PERSIST ============
$results | ForEach-Object {
    $s = if ($_.Pass) { 'PASS' } else { 'FAIL' }
    [string]::Join('|', @($s, $_.Section, $_.Test, $_.Expected, $_.Actual, $_.Note))
} | Set-Content 'D:\GOLANG\data\e2e_results.txt' -Encoding utf8
$fail = @($results | Where-Object { -not $_.Pass }).Count
Write-Host ("TOTAL={0} FAIL={1}" -f $results.Count, $fail)
if ($fail -gt 0) { $results | Where-Object { -not $_.Pass } | ForEach-Object { Write-Host ("FAIL: {0} :: {1} expected={2} actual={3} {4}" -f $_.Section, $_.Test, $_.Expected, $_.Actual, $_.Note) } }
