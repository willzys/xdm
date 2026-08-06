import { createChat } from "@xdevplatform/chat-xdk";

const chunks = [];
for await (const chunk of process.stdin) chunks.push(chunk);

let chat;
let pin;
try {
  const input = JSON.parse(Buffer.concat(chunks).toString("utf8"));
  pin = Uint8Array.from(Buffer.from(input.pin, "base64"));
  const tokens = new Map(
    Object.entries(input.realmTokens ?? {}).map(([key, value]) => [key.toLowerCase(), value]),
  );
  chat = await createChat({
    juiceboxConfig: JSON.stringify(input.juiceboxConfig),
    getAuthToken: async (realmId) => {
      const token = tokens.get(String(realmId).toLowerCase());
      if (!token) throw new Error(`missing Juicebox token for realm ${realmId}`);
      return token;
    },
  });
  await chat.unlock(pin);
  chat.setIdentity(input.userId, input.keyVersion);
  chat.setCacheKeys(true);
  chat.setSigningKeys(input.signingKeys ?? []);
  const result = chat.decryptEvents(input.events ?? []);
  const rows = Array.isArray(result?.messages) ? result.messages : [];
  const messages = rows.filter(
    (row) => String(row?.event?.type ?? "").toLowerCase() === "message",
  ).length;
  const errors = result?.errors && typeof result.errors === "object"
    ? Object.keys(result.errors).length
    : 0;
  process.stdout.write(JSON.stringify({ messages, events: rows.length, errors }));
} catch (error) {
  process.stdout.write(JSON.stringify({ error: error instanceof Error ? error.message : String(error) }));
  process.exitCode = 1;
} finally {
  pin?.fill(0);
  chat?.free();
}
