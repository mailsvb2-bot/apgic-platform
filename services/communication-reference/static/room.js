"use strict";
(() => {
  const status = document.getElementById("status");
  const tiles = document.getElementById("participants");
  const connectButton = document.getElementById("connect");
  const muteButton = document.getElementById("microphone");
  const cameraButton = document.getElementById("camera");
  const finishButton = document.getElementById("finish");
  const token = new URLSearchParams(location.hash.slice(1)).get("token");
  // Keep the short-lived media token out of history, referrers and logs.
  history.replaceState(null, "", location.pathname);
  let room = null;
  let finishing = false;
  let mediaURL = null;
  let startup = false;
  const peers = new Map();
  const setState = message => { status.textContent = message; };
  const removeTracks = () => {
    for (const element of peers.values()) {
      for (const child of element.querySelectorAll("audio,video")) {
        child.srcObject = null;
        child.remove();
      }
      element.remove();
    }
    peers.clear();
  };
  const tileFor = identity => {
    if (!peers.has(identity)) {
      const tile = document.createElement("div");
      tile.className = "media-tile";
      const title = document.createElement("p");
      title.textContent = identity.startsWith("SPECIALIST:") ? "Специалист" : "Клиент";
      tile.append(title);
      tiles.append(tile);
      peers.set(identity, tile);
    }
    return peers.get(identity);
  };
  function subscribe(track, publication, participant) {
    if (track.kind !== "audio" && track.kind !== "video") return;
    const container = tileFor(participant.identity);
    const element = track.attach();
    element.autoplay = true;
    element.playsInline = true;
    container.append(element);
  }
  function unsubscribe(track) {
    for (const element of track.detach()) element.remove();
  }
  function clean() {
    connectButton.disabled = false;
    muteButton.disabled = true;
    cameraButton.disabled = true;
    finishButton.disabled = true;
    removeTracks();
    room = null;
  }
  async function connect() {
    if (!token || !mediaURL || startup || room) return;
    const sdk = window.LivekitClient;
    if (!sdk?.Room || !sdk?.RoomEvent) {
      setState("Медиаклиент недоступен. Попробуйте позже.");
      return;
    }
    startup = true;
    connectButton.disabled = true;
    try {
      room = new sdk.Room({ adaptiveStream: true, dynacast: true });
      room.on(sdk.RoomEvent.TrackSubscribed, subscribe);
      room.on(sdk.RoomEvent.TrackUnsubscribed, unsubscribe);
      room.on(sdk.RoomEvent.Reconnecting, () => setState("Связь прервана. Восстанавливаем соединение…"));
      room.on(sdk.RoomEvent.Reconnected, () => setState("Соединение восстановлено."));
      room.on(sdk.RoomEvent.Disconnected, () => {
        if (!finishing) setState("Соединение завершено или утрачено. Вернитесь в APGIC для повторного входа.");
        clean();
      });
      await room.connect(mediaURL, token);
      try {
        await room.localParticipant.enableCameraAndMicrophone();
      } catch {
        // Audio-only is useful when camera is denied. Still require a real
        // published microphone track to claim local media is active.
        await room.localParticipant.setMicrophoneEnabled(true);
      }
      setState("Вы подключены к медиасервису. Ожидаем второго участника.");
      muteButton.disabled = false;
      cameraButton.disabled = false;
      finishButton.disabled = false;
      tileFor(room.localParticipant.identity);
    } catch {
      setState("Не удалось установить защищённое соединение. Вернитесь в APGIC для нового входа.");
      if (room) await room.disconnect().catch(() => {});
      clean();
    } finally {
      startup = false;
    }
  }
  async function finish() {
    if (!room || finishing) return;
    finishing = true;
    finishButton.disabled = true;
    try {
      const response = await fetch("/v1/rooms/finish", {
        method: "POST", headers: { Authorization: "Bearer " + token },
        credentials: "omit", cache: "no-store", referrerPolicy: "no-referrer",
      });
      if (!response.ok) throw new Error("finish intent not accepted");
      setState("Ваше завершение зафиксировано. Ожидаем подтверждение медиасервера.");
      await room.disconnect();
    } catch {
      finishing = false;
      finishButton.disabled = false;
      setState("Завершение не подтверждено. Не закрывайте страницу; повторите попытку.");
    }
  }
  connectButton.addEventListener("click", () => { void connect(); });
  finishButton.addEventListener("click", () => { void finish(); });
  muteButton.addEventListener("click", async () => {
    if (!room) return;
    const next = !room.localParticipant.isMicrophoneEnabled;
    await room.localParticipant.setMicrophoneEnabled(next);
    muteButton.textContent = next ? "Выключить микрофон" : "Включить микрофон";
  });
  cameraButton.addEventListener("click", async () => {
    if (!room) return;
    const next = !room.localParticipant.isCameraEnabled;
    await room.localParticipant.setCameraEnabled(next);
    cameraButton.textContent = next ? "Выключить камеру" : "Включить камеру";
  });
  if (!token) {
    setState("Ссылка доступа отсутствует или уже использована. Получите новую в APGIC.");
    connectButton.disabled = true;
    return;
  }
  fetch("/v1/media-config", { cache: "no-store", credentials: "omit" })
    .then(async response => {
      if (!response.ok) throw new Error("config denied");
      const payload = await response.json();
      if (typeof payload.server_url !== "string" || !payload.server_url.startsWith("wss://")) {
        throw new Error("invalid SFU");
      }
      mediaURL = payload.server_url;
      setState("Подключите микрофон и камеру, затем нажмите «Войти в консультацию».");
    }).catch(() => {
      connectButton.disabled = true;
      setState("Медиасервер не настроен.");
    });
})();
