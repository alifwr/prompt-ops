const fs = require('fs');

async function testChat() {
  const token = fs.readFileSync('jwt_token.txt', 'utf8');
  console.log("Sending chat message...");
  const res = await fetch("http://localhost:3001/api/chat", {
    method: "POST",
    headers: { 
      "Content-Type": "application/json",
      "Authorization": "Bearer " + token 
    },
    body: JSON.stringify({
      message: "Please backup my database.",
      server_id: 3,
      session_id: "test-sess-123"
    })
  });
  
  if (!res.ok) {
    console.error("Chat Error:", await res.text());
    return;
  }
  
  const text = await res.text();
  console.log("Chat Response:", text);
}

testChat().catch(console.error);
