$base = 'http://127.0.0.1:8082'
$secret = 'e2e2-proxy-secret-abcdef0123456789'

function RawPost($Url, $Text, $Ip, $Token) {
    if (-not $Token) { $Token = '' }
    $h = New-Object 'System.Collections.Generic.Dictionary[string,string]'
    $h['X-Nexa-Client-IP'] = $Ip
    $h['X-Nexa-Proxy-Secret'] = $secret
    $h['Content-Type'] = 'application/json'
    if ($Token -ne '') { $h['Authorization'] = 'Bearer ' + $Token }
    $p = [ordered]@{ Method = 'POST'; Uri = $Url; Headers = $h; UseBasicParsing = $true }
    if ($null -ne $Text) { $p['Body'] = $Text }
    $sw = [datetime]::now
    try {
        $r = Invoke-WebRequest @p
        return [pscustomobject]@{ Status = [int]$r.StatusCode; Ms = [int](([datetime]::now - $sw).TotalMilliseconds); Body = ($r.Content | ConvertFrom-Json) }
    } catch {
        $resp = $_.Exception.Response
        if ($resp) {
            $rd = [IO.StreamReader]::new($resp.GetResponseStream())
            $t = $rd.ReadToEnd()
            try { $j = $t | ConvertFrom-Json } catch { $j = @{ raw = $t } }
            return [pscustomobject]@{ Status = [int]$resp.StatusCode; Ms = [int](([datetime]::now - $sw).TotalMilliseconds); Body = $j }
        } else {
            return [pscustomobject]@{ Status = -1; Ms = 0; Body = @{ raw = $_.Exception.Message } }
        }
    }
}

$pad = 'x' * 70000
$r = RawPost "$base/api/v1/auth/login" ('{"email":"a@b.test","padding":"' + $pad + '"}') '10.13.1.1' ''
Write-Host ("1 OVERSIZED BODY(70KB): status={0} (expect 400)" -f $r.Status)

$r = RawPost "$base/api/v1/auth/login" '{"email": bad json' '10.13.1.2' ''
Write-Host ("2 MALFORMED JSON: status={0} (expect 400)" -f $r.Status)

$r = RawPost "$base/api/v1/auth/login" '{"a":1} {"b":2}' '10.13.1.3' ''
Write-Host ("3 DOUBLE OBJECT: status={0} (expect 400)" -f $r.Status)

$sw1 = [datetime]::now
foreach ($i in 1..5) { $null = RawPost "$base/api/v1/auth/login" ('{"email":"ghost999' + $i + '@x.test","password":"password-123456"}') '10.13.2.1' '' }
$msGhost = [int](([datetime]::now - $sw1).TotalMilliseconds)
$null = RawPost "$base/api/v1/auth/register" '{"name":"Tk","email":"timed@x.test","password":"password-123456"}' '10.13.2.2' ''
$sw2 = [datetime]::now
foreach ($i in 1..5) { $null = RawPost "$base/api/v1/auth/login" '{"email":"timed@x.test","password":"totally-wrong-1"}' '10.13.2.3' '' }
$msKnown = [int](([datetime]::now - $sw2).TotalMilliseconds)
Write-Host ("4 TIMING: unknown(5x)={0}ms vs known(5x)={1}ms (should be close)" -f $msGhost, $msKnown)

$null = RawPost "$base/api/v1/auth/register" '{"name":"Lk9","email":"lk9@x.test","password":"password-123456"}' '10.13.3.1' ''
foreach ($i in 1..5) { $null = RawPost "$base/api/v1/auth/login" '{"email":"lk9@x.test","password":"nope-12345"}' '10.13.3.2' '' }
$r = RawPost "$base/api/v1/auth/login" '{"email":"lk9@x.test","password":"nope-12345"}' '10.13.3.2' ''
Write-Host ("5 LOCKED MSG: status={0} msg='{1}'" -f $r.Status, $r.Body.message)

try {
    $g = Invoke-WebRequest "$base/api/v1/auth/register" -Headers @{'X-Nexa-Client-IP'='10.13.4.1';'X-Nexa-Proxy-Secret'=$secret} -UseBasicParsing
    Write-Host ("6 GET REGISTER: status={0} (expect 405)" -f $g.StatusCode)
} catch {
    Write-Host ("6 GET REGISTER: status={0} (expect 405)" -f [int]$_.Exception.Response.StatusCode)
}

try {
    $w = Invoke-WebRequest "$base/api/v1/auth/login" -Method POST -Body '{"email":"a@b.test","password":"password-123456"}' -Headers @{'X-Nexa-Client-IP'='10.13.4.2';'X-Nexa-Proxy-Secret'=$secret;'Content-Type'='text/plain'} -UseBasicParsing
    Write-Host ("7 TEXT-PLAIN: status={0} (expect 415)" -f $w.StatusCode)
} catch {
    Write-Host ("7 TEXT-PLAIN: status={0} (expect 415)" -f [int]$_.Exception.Response.StatusCode)
}

$r = RawPost "$base/api/v1/auth/login" '{"email":"ghost@x.test"}' '10.13.4.3' ''
Write-Host ("8 EMPTY PASSWORD UNKNOWN EMAIL: status={0} (expect 401) msg='{1}'" -f $r.Status, $r.Body.message)
