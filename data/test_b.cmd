@echo off
echo 5 wrong attempts:
for /L %%i in (1,1,5) do @curl -s -o NUL -X POST http://127.0.0.1:8082/api/v1/auth/login -H "Content-Type: application/json" -H "X-Nexa-Client-IP: 10.12.1.3" -H "X-Nexa-Proxy-Secret: e2e2-proxy-secret-abcdef0123456789" -d "{\"email\":\"lk2@e2e2.test\",\"password\":\"wrong-wrong-1234\"}"
echo correct pw while locked (RAW):
curl -s -X POST http://127.0.0.1:8082/api/v1/auth/login -H "Content-Type: application/json" -H "X-Nexa-Client-IP: 10.12.1.3" -H "X-Nexa-Proxy-Secret: e2e2-proxy-secret-abcdef0123456789" -d "{\"email\":\"lk2@e2e2.test\",\"password\":\"password-123456\"}"
echo.
echo === security headers (RAW) ===
curl -s -D - -o NUL http://127.0.0.1:8082/api/v1/auth/me
echo === unknown endpoint ===
curl -s -o NUL -w "unknown=%{http_code}\n" http://127.0.0.1:8082/api/v1/does-not-exist
