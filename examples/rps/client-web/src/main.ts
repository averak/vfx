import { VfxClient, type Frame, type Session } from "@averak/vfx-client";

const gatewayUrl = import.meta.env.VITE_GATEWAY_URL ?? "http://localhost:8080";

const statusEl = document.querySelector<HTMLParagraphElement>("#status")!;
const choicesEl = document.querySelector<HTMLDivElement>("#choices")!;
const stateEl = document.querySelector<HTMLPreElement>("#state")!;
const logEl = document.querySelector<HTMLPreElement>("#log")!;

function log(message: string): void {
  logEl.textContent += `${message}\n`;
}

function setStatus(message: string): void {
  statusEl.textContent = message;
}

let tick = 0;

async function main(): Promise<void> {
  const client = new VfxClient(gatewayUrl);

  // A stable device id per browser keeps the same guest identity across
  // reloads. localStorage is the simplest place to keep it.
  let deviceId = localStorage.getItem("vfx_device_id");
  if (!deviceId) {
    deviceId = crypto.randomUUID();
    localStorage.setItem("vfx_device_id", deviceId);
  }

  const player = await client.loginAnonymous(deviceId, "WebPlayer");
  log(`logged in as ${player.id}`);

  const ticketId = await client.createTicket("rps");
  log(`ticket ${ticketId} queued`);
  setStatus("Waiting for an opponent…");

  const match = await client.waitForMatch(ticketId);
  log(`matched! endpoint=${match.endpoint}`);
  setStatus("Matched — connecting to room…");

  // NOTE: browser WebTransport requires a certificate the browser
  // trusts. Use mkcert for local development so no serverCertificateHashes
  // are needed. (RSA dev certs are not eligible for the hash-pinning API.)
  const session = await match.connect();
  setStatus("Connected — pick your move!");
  choicesEl.hidden = false;

  session.onFrame((frame: Frame) => handleFrame(frame));
  wireButtons(session);
}

function handleFrame(frame: Frame): void {
  switch (frame.body.case) {
    case "delta": {
      const text = new TextDecoder().decode(frame.body.value.payload);
      try {
        stateEl.textContent = JSON.stringify(JSON.parse(text), null, 2);
      } catch {
        stateEl.textContent = text;
      }
      break;
    }
    case "event":
      log(`event ${frame.body.value.type}`);
      break;
    case "error":
      log(`server error ${frame.body.value.code}: ${frame.body.value.message}`);
      break;
  }
}

function wireButtons(session: Session): void {
  for (const button of document.querySelectorAll<HTMLButtonElement>("#choices button")) {
    button.addEventListener("click", () => {
      const choice = button.dataset.choice ?? "R";
      void session.sendInput(tick, new TextEncoder().encode(choice));
      log(`sent ${choice}`);
      tick += 1;
    });
  }
}

main().catch((err: unknown) => {
  setStatus(`Error: ${String(err)}`);
  log(String(err));
});
