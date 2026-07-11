const fs = require('fs');

async function testAll() {
  console.log("1. Testing Registration...");
  let res = await fetch("http://localhost:3001/api/auth/register", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email: "test2@example.com", password: "password123" })
  });
  let data = await res.json();
  console.log("Register Response:", data);

  console.log("\n2. Testing Login...");
  res = await fetch("http://localhost:3001/api/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email: "test2@example.com", password: "password123" })
  });
  data = await res.json();
  console.log("Login Response (has token?):", !!data.token);
  const token = data.token;

  console.log("\n3. Testing Token Generation (Chat flow proxy)...");
  res = await fetch("http://localhost:3001/api/servers/generate-token", {
    method: "POST",
    headers: { "Content-Type": "application/json", "Authorization": "Bearer " + token },
    body: JSON.stringify({ name: "my-test-vps" })
  });
  data = await res.json();
  console.log("Generate Token Response:", data.token);
  const vpsToken = data.token;

  console.log("\n4. Retrieving server list (Should be registering)...");
  res = await fetch("http://localhost:3001/api/servers", {
    headers: { "Authorization": "Bearer " + token }
  });
  let servers = await res.json();
  console.log("Servers:", servers);

  fs.writeFileSync('test_token.txt', vpsToken);
  fs.writeFileSync('jwt_token.txt', token);
  console.log("\nSaved tokens for daemon test.");
}

testAll().catch(console.error);
