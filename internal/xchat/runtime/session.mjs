import { createInterface } from "node:readline";
import { createChat } from "@xdevplatform/chat-xdk";

let chat;

function eventPage(result) {
  const events = [];
  let messageEvents = 0;
  for (const row of Array.isArray(result?.messages) ? result.messages : []) {
    const event = row?.event;
    if (String(event?.type ?? "").toLowerCase() !== "message") continue;
    messageEvents++;
    const contentType = String(event?.content?.contentType ?? "").toLowerCase();
    if (contentType && contentType !== "text") continue;
    if (typeof event?.content?.text !== "string") continue;
    events.push({
      id: String(event.id ?? event.sequenceId ?? ""),
      senderId: String(event.senderId ?? ""),
      conversationId: String(event.conversationId ?? ""),
      createdAtMsec: Number(event.createdAtMsec ?? 0),
      text: String(event.content?.text ?? ""),
      verified: Boolean(event.verified),
    });
  }
  const errors = result?.errors && typeof result.errors === "object"
    ? Object.keys(result.errors).length
    : 0;
  return { events, messageEvents, errors };
}

async function unlock(input) {
  const pin = Uint8Array.from(Buffer.from(input.pin, "base64"));
  const tokens = new Map(
    Object.entries(input.material.realmTokens ?? {}).map(([key, value]) => [key.toLowerCase(), value]),
  );
  try {
    chat = await createChat({
      juiceboxConfig: JSON.stringify(input.material.juiceboxConfig),
      getAuthToken: async (realmId) => {
        const token = tokens.get(String(realmId).toLowerCase());
        if (!token) throw new Error(`missing Juicebox token for realm ${realmId}`);
        return token;
      },
    });
    await chat.unlock(pin);
    chat.setIdentity(input.material.userId, input.material.keyVersion);
    chat.setCacheKeys(true);
    chat.setSigningKeys(input.material.signingKeys ?? []);
    return { ready: true, events: [], messageEvents: 0, errors: 0 };
  } finally {
    pin.fill(0);
  }
}

function decrypt(input) {
  if (!chat) throw new Error("XChat session is locked");
  chat.setSigningKeys(input.material.signingKeys ?? []);
  return eventPage(chat.decryptEvents(input.material.events ?? []));
}

const lines = createInterface({ input: process.stdin, crlfDelay: Infinity });
for await (const line of lines) {
  try {
    const input = JSON.parse(line);
    if (input.op === "close") break;
    const result = input.op === "unlock" ? await unlock(input) : decrypt(input);
    process.stdout.write(`${JSON.stringify(result)}\n`);
  } catch (error) {
    process.stdout.write(`${JSON.stringify({ error: error instanceof Error ? error.message : String(error) })}\n`);
  }
}

chat?.free();
