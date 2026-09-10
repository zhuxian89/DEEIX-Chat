import { useCallback, useEffect, useRef, useState } from "react";
import Taro, { useDidShow } from "@tarojs/taro";
import { createTaroTransport } from "../../platform/transport";
import { todoCache } from "../../platform/todo-cache";
import { resolveMiniAppConfig } from "../runtime-config";
import { TodoClient } from "./client";
import { TodoStore } from "./store";
import { newID, type Operation, type Snapshot } from "./types";

export const aiPage = "/pages/index/index";
export const messageOf = (error: unknown): string => error instanceof Error ? error.message : "暂时无法连接，请稍后重试";

export function useTodo() {
  const [loading, setLoading] = useState(true);
  const [syncing, setSyncing] = useState(false);
  const [error, setError] = useState("");
  const [online, setOnline] = useState(true);
  const [snapshot, setSnapshot] = useState<Snapshot>({ lists: [], tasks: [] });
  const [, render] = useState(0);
  const client = useRef<TodoClient>();
  const store = useRef<TodoStore>();
  const busy = useRef(false);
  const connecting = useRef(false);
  const alive = useRef(true);
  const redraw = useCallback(() => {
    if (!alive.current) return;
    if (store.current) setSnapshot(store.current.view);
    render((value) => value + 1);
  }, []);
  const sync = useCallback(async () => {
    const active = store.current;
    const api = client.current;
    if (!active || !api || busy.current || connecting.current) return;
    busy.current = true; setSyncing(true);
    try {
      const status = await api.status();
      if (status.ownerKey !== active.ownerKey) throw new Error("微信身份已变化，请重新连接；原待办已单独保留");
      if (status.unlocked) { await Taro.reLaunch({ url: aiPage }); return; }
      while (active.pending.length && !active.pending[0].problem && alive.current) {
        const result = await api.sync([active.pending[0].operation]);
        active.accept(result); redraw();
      }
      if (!active.pending.length) active.replaceSnapshot(await api.snapshot());
      setOnline(true); setError(""); redraw();
    } catch (failure) {
      if (alive.current) { setOnline(false); setError(messageOf(failure)); }
    } finally { busy.current = false; if (alive.current) setSyncing(false); }
  }, [redraw]);
  const connect = useCallback(async () => {
    if (connecting.current || busy.current) return;
    connecting.current = true; setLoading(true); setError("");
    try {
      client.current?.dispose();
      const { apiBaseUrl } = resolveMiniAppConfig(process.env.TARO_APP_API_BASE_URL);
      const api = new TodoClient(createTaroTransport(apiBaseUrl), async () => (await Taro.login({ timeout: 10000 })).code);
      client.current = api;
      const status = await api.connect();
      if (!alive.current) return;
      if (status.unlocked) { await Taro.reLaunch({ url: aiPage }); return; }
      store.current = new TodoStore(status.ownerKey, todoCache);
      redraw(); setOnline(true);
    } catch (failure) {
      // Never open another identity's cache while login is unresolved.
      store.current = undefined; setSnapshot({ lists: [], tasks: [] });
      if (alive.current) { setError(messageOf(failure)); setOnline(false); }
    } finally {
      connecting.current = false;
      if (alive.current) { setLoading(false); void sync(); }
    }
  }, [redraw, sync]);
  useEffect(() => {
    alive.current = true;
    void connect();
    const onNetwork = (event: { isConnected: boolean }) => {
      if (event.isConnected) { if (store.current) void sync(); else void connect(); }
      else setOnline(false);
    };
    Taro.onNetworkStatusChange(onNetwork);
    return () => { alive.current = false; Taro.offNetworkStatusChange(onNetwork); client.current?.dispose(); };
  }, [connect, sync]);
  useDidShow(() => { void sync(); });
  const activeStore = store.current;
  const enqueue = useCallback((operation: Omit<Operation, "id">) => {
    if (!activeStore || store.current !== activeStore) throw new Error("微信身份已重新确认，请重新操作");
    activeStore.enqueue({ ...operation, id: newID() });
    redraw(); void sync();
  }, [activeStore, redraw, sync]);
  return { loading, syncing, error, online, snapshot, client, store, enqueue, redraw, sync, connect };
}
