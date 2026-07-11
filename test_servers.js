const fs = require('fs');

async function checkServers() {
  const token = fs.readFileSync('jwt_token.txt', 'utf8');
  console.log("Retrieving server list (Should be online)...");
  const res = await fetch("http://localhost:3001/api/servers", {
    headers: { "Authorization": "Bearer " + token }
  });
  let servers = await res.json();
  console.log("Servers:", servers);
}

checkServers().catch(console.error);
