// TypeScript client SDK for vfx.
//
// Mirrors the Go SDK (sdk/client/go) so the same flow reads the same in both languages:
//
//   const client = new VfxClient("http://localhost:8080");
//   await client.loginAnonymous("device-1", "Alice");
//   const ticketId = await client.createTicket("rps");
//   const match = await client.waitForMatch(ticketId);
//   const session = await match.connect();
//   session.onFrame((frame) => { ... });
//   await session.sendInput(0, new Uint8Array([82])); // 'R'
//
// Realtime uses the browser-native WebTransport API.
// For local development against a self-signed certificate, pass the cert's SHA-256 hash via connect({ serverCertificateHashes }).

import { create, fromBinary, toBinary } from "@bufbuild/protobuf";
import { createClient, type Client } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

import { AuthService, Provider, type Player } from "./gen/vfx/v1/auth/auth_service_pb.js";
import { MatchService } from "./gen/vfx/v1/match/match_service_pb.js";
import {
  FrameSchema,
  type Frame,
  PlayerInputSchema,
  ClientHelloSchema,
} from "./gen/vfx/v1/realtime/frame_pb.js";
import {
  PlayerDataStorageService,
  TitleStorageService,
  type FileMetadata,
} from "./gen/vfx/v1/storage/storage_service_pb.js";
import { LeaderboardService, type RankEntry } from "./gen/vfx/v1/leaderboard/leaderboard_service_pb.js";
import {
  SocialService,
  RequestStatus,
  type Friend,
  type FriendRequest,
  type BlockedPlayer,
} from "./gen/vfx/v1/social/social_service_pb.js";
import { ChatService, type Message as ChatMessage } from "./gen/vfx/v1/chat/chat_service_pb.js";
import { GroupService, type Group, type Member } from "./gen/vfx/v1/group/group_service_pb.js";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";

export { Provider, RequestStatus };
export type { Player, Frame, FileMetadata, RankEntry, Friend, FriendRequest, BlockedPlayer, ChatMessage, Group, Member };

/** Options for constructing a VfxClient. */
export interface VfxClientOptions {
  /** Override the fetch implementation (e.g. for tests). */
  fetch?: typeof globalThis.fetch;
}

/** A logged-in (or about-to-be) vfx client. Create one per player. */
export class VfxClient {
  private readonly auth: Client<typeof AuthService>;
  private readonly match: Client<typeof MatchService>;
  private readonly playerData: Client<typeof PlayerDataStorageService>;
  private readonly titleStorage: Client<typeof TitleStorageService>;
  private readonly leaderboard: Client<typeof LeaderboardService>;
  private readonly social: Client<typeof SocialService>;
  private readonly chat: Client<typeof ChatService>;
  private readonly group: Client<typeof GroupService>;
  // The same fetch is used for the direct object-store transfers (PUT/GET on signed URLs), which do not go through the Connect transport.
  private readonly fetchImpl: typeof globalThis.fetch;
  private accessToken = "";
  private player?: Player;

  constructor(gatewayUrl: string, options: VfxClientOptions = {}) {
    const transport = createConnectTransport({
      baseUrl: gatewayUrl,
      fetch: options.fetch,
    });
    this.auth = createClient(AuthService, transport);
    this.match = createClient(MatchService, transport);
    this.playerData = createClient(PlayerDataStorageService, transport);
    this.titleStorage = createClient(TitleStorageService, transport);
    this.leaderboard = createClient(LeaderboardService, transport);
    this.social = createClient(SocialService, transport);
    this.chat = createClient(ChatService, transport);
    this.group = createClient(GroupService, transport);
    this.fetchImpl = options.fetch ?? globalThis.fetch.bind(globalThis);
  }

  /** The authenticated player, or undefined before login. */
  getPlayer(): Player | undefined {
    return this.player;
  }

  /**
   * Log in with an anonymous credential.
   * A stable deviceId returns the same player across calls; an empty deviceId mints a fresh one.
   * nickname is applied only on first registration.
   */
  async loginAnonymous(deviceId?: string, nickname?: string): Promise<Player> {
    const resp = await this.auth.login({
      credential: {
        case: "anonymous",
        value: { deviceId, nickname },
      },
    });
    this.accessToken = resp.accessToken;
    this.player = resp.player;
    if (!this.player) {
      throw new Error("vfx: login response missing player");
    }
    return this.player;
  }

  /** Log in with a provider ID token (Google or Apple); the same provider account always returns the same player. */
  async loginOidc(provider: Provider, idToken: string): Promise<Player> {
    const resp = await this.auth.login({ credential: { case: "oidc", value: { provider, idToken } } });
    this.accessToken = resp.accessToken;
    this.player = resp.player;
    if (!this.player) {
      throw new Error("vfx: login response missing player");
    }
    return this.player;
  }

  /** Attach a provider identity to the current player, upgrading an anonymous account. */
  async linkIdentity(provider: Provider, idToken: string): Promise<Player> {
    const resp = await this.auth.linkIdentity(
      { oidc: { provider, idToken } },
      { headers: this.authHeaders() },
    );
    this.player = resp.player;
    if (!this.player) {
      throw new Error("vfx: link response missing player");
    }
    return this.player;
  }

  /** Enqueue a matchmaking ticket and return its id. */
  async createTicket(gameMode: string): Promise<string> {
    const resp = await this.match.createTicket(
      { gameMode },
      { headers: this.authHeaders() },
    );
    return resp.ticketId;
  }

  /**
   * Follow the WatchTicket stream until the ticket is matched, then resolve with the connection details.
   * Rejects on matchmaking failure.
   */
  async waitForMatch(ticketId: string): Promise<Match> {
    const stream = this.match.watchTicket(
      { ticketId },
      { headers: this.authHeaders() },
    );
    for await (const msg of stream) {
      switch (msg.event.case) {
        case "queued":
          break;
        case "matched":
          return new Match(msg.event.value.endpoint, msg.event.value.sessionToken);
        case "failed":
          throw new Error(
            `vfx: matchmaking failed: ${msg.event.value.reason} (${msg.event.value.message})`,
          );
      }
    }
    throw new Error("vfx: ticket stream closed without a match");
  }

  /** List the player's stored files with metadata, for diff-sync against local copies via the hash field. */
  async queryFiles(prefix = ""): Promise<FileMetadata[]> {
    const resp = await this.playerData.queryFiles({ prefix }, { headers: this.authHeaders() });
    return resp.files;
  }

  /**
   * Store data under filename, hiding the two-step upload: request an upload URL, PUT the bytes directly to the object store, then commit so the gateway records the verified metadata.
   */
  async writeFile(filename: string, data: Uint8Array): Promise<void> {
    const resp = await this.playerData.writeFile(
      { filename, size: BigInt(data.length) },
      { headers: this.authHeaders() },
    );
    await this.putObject(resp.uploadUrl, resp.requiredHeaders, data);
    await this.playerData.commitFile({ filename }, { headers: this.authHeaders() });
  }

  /** Fetch filename's bytes, hiding the URL step: ask for a download URL and GET the bytes directly from the object store. */
  async readFile(filename: string): Promise<Uint8Array> {
    const resp = await this.playerData.readFile({ filename }, { headers: this.authHeaders() });
    return this.getObject(resp.downloadUrl);
  }

  async deleteFile(filename: string): Promise<void> {
    await this.playerData.deleteFile({ filename }, { headers: this.authHeaders() });
  }

  /** List operator-published title files carrying all of the given tags (no tags lists everything visible). */
  async queryTitleFiles(tags: string[] = []): Promise<FileMetadata[]> {
    const resp = await this.titleStorage.queryFiles({ tags }, { headers: this.authHeaders() });
    return resp.files;
  }

  async readTitleFile(filename: string): Promise<Uint8Array> {
    const resp = await this.titleStorage.readFile({ filename }, { headers: this.authHeaders() });
    return this.getObject(resp.downloadUrl);
  }

  /** Submit a score; resolves with the player's resulting entry and whether it improved their best (keep-best). */
  async submitScore(leaderboardId: string, score: bigint): Promise<{ entry?: RankEntry; improved: boolean }> {
    const resp = await this.leaderboard.submitScore({ leaderboardId, score }, { headers: this.authHeaders() });
    return { entry: resp.entry, improved: resp.improved };
  }

  async listRanks(leaderboardId: string, offset = 0, limit = 0): Promise<RankEntry[]> {
    const resp = await this.leaderboard.listRanks({ leaderboardId, offset, limit }, { headers: this.authHeaders() });
    return resp.entries;
  }

  /** The authenticated player's rank, or the given player's when playerId is set. */
  async getPlayerRank(leaderboardId: string, playerId?: string): Promise<RankEntry | undefined> {
    const resp = await this.leaderboard.getPlayerRank({ leaderboardId, playerId }, { headers: this.authHeaders() });
    return resp.entry;
  }

  async listRanksAroundPlayer(leaderboardId: string, radius: number): Promise<RankEntry[]> {
    const resp = await this.leaderboard.listRanksAroundPlayer({ leaderboardId, radius }, { headers: this.authHeaders() });
    return resp.entries;
  }

  private async putObject(
    url: string,
    headers: Record<string, string>,
    data: Uint8Array,
  ): Promise<void> {
    // Copy into a fresh ArrayBuffer-backed view so the body is a plain BufferSource regardless of the input's backing store.
    const body = new Uint8Array(data.length);
    body.set(data);
    const resp = await this.fetchImpl(url, { method: "PUT", headers, body });
    if (!resp.ok) {
      throw new Error(`vfx: upload returned ${resp.status}`);
    }
  }

  private async getObject(url: string): Promise<Uint8Array> {
    const resp = await this.fetchImpl(url);
    if (!resp.ok) {
      throw new Error(`vfx: download returned ${resp.status}`);
    }
    return new Uint8Array(await resp.arrayBuffer());
  }

  /** Send a friend request; the result is ACCEPTED when the addressee already had a pending request to you (mutual), otherwise PENDING. */
  async sendFriendRequest(addresseePlayerId: string): Promise<RequestStatus> {
    const resp = await this.social.sendFriendRequest({ addresseePlayerId }, { headers: this.authHeaders() });
    return resp.status;
  }

  async acceptFriendRequest(requesterPlayerId: string): Promise<void> {
    await this.social.acceptFriendRequest({ requesterPlayerId }, { headers: this.authHeaders() });
  }

  async declineFriendRequest(requesterPlayerId: string): Promise<void> {
    await this.social.declineFriendRequest({ requesterPlayerId }, { headers: this.authHeaders() });
  }

  async cancelFriendRequest(addresseePlayerId: string): Promise<void> {
    await this.social.cancelFriendRequest({ addresseePlayerId }, { headers: this.authHeaders() });
  }

  async listFriends(): Promise<Friend[]> {
    const resp = await this.social.listFriends({}, { headers: this.authHeaders() });
    return resp.friends;
  }

  async listIncomingFriendRequests(): Promise<FriendRequest[]> {
    const resp = await this.social.listIncomingRequests({}, { headers: this.authHeaders() });
    return resp.requests;
  }

  async listOutgoingFriendRequests(): Promise<FriendRequest[]> {
    const resp = await this.social.listOutgoingRequests({}, { headers: this.authHeaders() });
    return resp.requests;
  }

  async removeFriend(friendPlayerId: string): Promise<void> {
    await this.social.removeFriend({ friendPlayerId }, { headers: this.authHeaders() });
  }

  /** Block a player, severing any friendship and pending requests; idempotent. */
  async blockPlayer(playerId: string): Promise<void> {
    await this.social.blockPlayer({ playerId }, { headers: this.authHeaders() });
  }

  async unblockPlayer(playerId: string): Promise<void> {
    await this.social.unblockPlayer({ playerId }, { headers: this.authHeaders() });
  }

  async listBlocked(): Promise<BlockedPlayer[]> {
    const resp = await this.social.listBlocked({}, { headers: this.authHeaders() });
    return resp.blocked;
  }

  /** Send a direct message and return the stored message. */
  async sendDirectMessage(recipientId: string, body: string): Promise<ChatMessage | undefined> {
    const resp = await this.chat.sendDirectMessage({ recipientId, body }, { headers: this.authHeaders() });
    return resp.message;
  }

  /** Conversation history with another player, newest-first; pass before to page back to older messages. */
  async listDirectMessages(
    otherPlayerId: string,
    opts: { before?: Date; limit?: number } = {},
  ): Promise<ChatMessage[]> {
    const resp = await this.chat.listDirectMessages(
      {
        otherPlayerId,
        before: opts.before ? timestampFromDate(opts.before) : undefined,
        limit: opts.limit ?? 0,
      },
      { headers: this.authHeaders() },
    );
    return resp.messages;
  }

  /** Create a group with the caller as owner and first member. */
  async createGroup(name: string): Promise<Group | undefined> {
    const resp = await this.group.createGroup({ name }, { headers: this.authHeaders() });
    return resp.group;
  }

  /** Disband a group the caller owns. */
  async deleteGroup(groupId: string): Promise<void> {
    await this.group.deleteGroup({ groupId }, { headers: this.authHeaders() });
  }

  async joinGroup(groupId: string): Promise<void> {
    await this.group.joinGroup({ groupId }, { headers: this.authHeaders() });
  }

  async leaveGroup(groupId: string): Promise<void> {
    await this.group.leaveGroup({ groupId }, { headers: this.authHeaders() });
  }

  async listMyGroups(): Promise<Group[]> {
    const resp = await this.group.listMyGroups({}, { headers: this.authHeaders() });
    return resp.groups;
  }

  async listGroupMembers(groupId: string): Promise<Member[]> {
    const resp = await this.group.listMembers({ groupId }, { headers: this.authHeaders() });
    return resp.members;
  }

  private authHeaders(): HeadersInit {
    return this.accessToken ? { Authorization: `Bearer ${this.accessToken}` } : {};
  }
}

/** Options for opening a room session. */
export interface ConnectOptions {
  /**
   * SHA-256 hashes of acceptable server certificates, for connecting to a self-signed development server.
   * Each entry is the raw 32-byte digest.
   * When omitted the browser's normal CA validation applies.
   */
  serverCertificateHashes?: Uint8Array[];
}

/** The result of a successful match: where to connect and the token. */
export class Match {
  constructor(
    readonly endpoint: string,
    readonly sessionToken: string,
  ) {}

  /** Open a WebTransport session to the matched room. */
  async connect(options: ConnectOptions = {}): Promise<Session> {
    const matchId = matchIdFromToken(this.sessionToken);
    const url = `https://${this.endpoint}/room/${matchId}`;

    const init: WebTransportOptions = {};
    if (options.serverCertificateHashes) {
      init.serverCertificateHashes = options.serverCertificateHashes.map((hash) => {
        // Copy into a fresh ArrayBuffer-backed view so the type is BufferSource regardless of where the bytes originated.
        const value = new Uint8Array(hash.length);
        value.set(hash);
        return { algorithm: "sha-256", value };
      });
    }

    const wt = new WebTransport(url, init);
    await wt.ready;

    // Authenticate with a ClientHello before anything else: the browser WebTransport API cannot set the Authorization header on the CONNECT, so the token rides in the first reliable frame.
    const hello = create(FrameSchema, {
      body: { case: "hello", value: create(ClientHelloSchema, { sessionToken: this.sessionToken }) },
    });
    const stream = await wt.createUnidirectionalStream();
    const writer = stream.getWriter();
    await writer.write(toBinary(FrameSchema, hello));
    await writer.close();

    return new Session(wt, this.sessionToken);
  }
}

/** A live WebTransport connection to a room. */
export class Session {
  private readonly writer: WritableStreamDefaultWriter<Uint8Array>;
  private readonly reader: ReadableStreamDefaultReader<Uint8Array>;
  private readonly frameHandlers: ((frame: Frame) => void)[] = [];
  private closed = false;

  constructor(
    private readonly wt: WebTransport,
    readonly sessionToken: string,
  ) {
    this.writer = wt.datagrams.writable.getWriter();
    this.reader = wt.datagrams.readable.getReader();
    void this.readLoop();
    void this.streamLoop();
  }

  /** Register a callback invoked for every inbound frame. */
  onFrame(handler: (frame: Frame) => void): void {
    this.frameHandlers.push(handler);
  }

  /** Send a PlayerInput frame as an unreliable datagram. */
  async sendInput(tick: number, payload: Uint8Array): Promise<void> {
    const frame = create(FrameSchema, {
      body: {
        case: "input",
        value: create(PlayerInputSchema, { tick, payload }),
      },
    });
    await this.writer.write(toBinary(FrameSchema, frame));
  }

  /** Close the session. */
  close(): void {
    if (this.closed) return;
    this.closed = true;
    this.wt.close();
  }

  private async readLoop(): Promise<void> {
    try {
      for (;;) {
        const { value, done } = await this.reader.read();
        if (done) return;
        if (!value) continue;
        this.dispatch(value);
      }
    } catch {
      // Stream closed; nothing actionable.
    }
  }

  // streamLoop reads frames sent over reliable unidirectional streams (large snapshots); each stream carries exactly one frame.
  private async streamLoop(): Promise<void> {
    try {
      const streams = this.wt.incomingUnidirectionalStreams.getReader();
      for (;;) {
        const { value: stream, done } = await streams.read();
        if (done) return;
        if (!stream) continue;
        void this.readStream(stream);
      }
    } catch {
      // Session closed; nothing actionable.
    }
  }

  private async readStream(stream: ReadableStream<Uint8Array>): Promise<void> {
    try {
      const reader = stream.getReader();
      const chunks: Uint8Array[] = [];
      for (;;) {
        const { value, done } = await reader.read();
        if (done) break;
        if (value) chunks.push(value);
      }
      this.dispatch(concatChunks(chunks));
    } catch {
      // Malformed or interrupted stream; drop the frame.
    }
  }

  private dispatch(data: Uint8Array): void {
    const frame = fromBinary(FrameSchema, data);
    for (const handler of this.frameHandlers) {
      handler(frame);
    }
  }
}

function concatChunks(chunks: Uint8Array[]): Uint8Array {
  let total = 0;
  for (const c of chunks) total += c.length;
  const out = new Uint8Array(total);
  let offset = 0;
  for (const c of chunks) {
    out.set(c, offset);
    offset += c.length;
  }
  return out;
}

/**
 * Pull the match id ("mid" claim) out of the session token's JWT payload.
 * The client does not verify the signature (the room does that on accept); it only needs the id to build the URL.
 */
function matchIdFromToken(token: string): string {
  const parts = token.split(".");
  if (parts.length !== 3) {
    throw new Error("vfx: malformed session token");
  }
  const payload = JSON.parse(base64UrlDecode(parts[1])) as { mid?: string };
  if (!payload.mid) {
    throw new Error("vfx: session token has no match id");
  }
  return payload.mid;
}

function base64UrlDecode(segment: string): string {
  const padded = segment.replace(/-/g, "+").replace(/_/g, "/");
  const withPad = padded + "=".repeat((4 - (padded.length % 4)) % 4);
  return atob(withPad);
}
