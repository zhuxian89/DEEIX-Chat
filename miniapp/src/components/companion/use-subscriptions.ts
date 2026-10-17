import Taro from "@tarojs/taro";
import { useEffect, useMemo, useState } from "react";
import type { CompanionClient } from "@/product/companion-client";
import { CompanionSubscriptions } from "@/product/companion-subscriptions";

const subscriptions = new WeakMap<CompanionClient, CompanionSubscriptions>();
function forClient(client: CompanionClient): CompanionSubscriptions {
  let controller = subscriptions.get(client);
  if (!controller) {
    controller = new CompanionSubscriptions(client, {
      choices: async () => (await Taro.getSetting({ withSubscriptions: true })).subscriptionsSetting ?? {},
      request: async (id) => {
        const result = await Taro.requestSubscribeMessage({ tmplIds: [id] } as Taro.requestSubscribeMessage.Option);
        const choice: unknown = Reflect.get(result, id);
        return typeof choice === "string" ? choice : "reject";
      },
    });
    subscriptions.set(client, controller);
  }
  return controller;
}

export function useCompanionSubscriptions(client: CompanionClient) {
  const controller = useMemo(() => forClient(client), [client]);
  const [state, setState] = useState(controller.state);
  useEffect(() => {
    const unsubscribe = controller.subscribe(setState);
    const show = () => {
      // A grant may finish while hidden. Discard its saved choice before the
      // first foreground click, in case preferences changed outside the app.
      if (controller.state.remembered) controller.invalidate();
      void controller.refresh();
    };
    const hide = () => controller.invalidate();
    Taro.onAppShow(show);
    Taro.onAppHide(hide);
    void controller.refresh();
    return () => { unsubscribe(); Taro.offAppShow(show); Taro.offAppHide(hide); };
  }, [controller]);
  return { controller, ...state };
}
