import Taro from "@tarojs/taro";
import type { CacheStorage } from "../product/todo/store";
import { newID } from "../product/todo/types";

// The small storage entry is an atomic pointer. Task bodies live in a file,
// avoiding WeChat's 1 MB per-key limit. A failed write keeps the previous file.
export const todoCache: CacheStorage = {
  read(key) {
    const filename = Taro.getStorageSync<string>(key);
    if (!filename) return undefined;
    if (typeof filename !== "string" || !/^todo-[a-zA-Z0-9_-]+\.json$/.test(filename)) throw new Error("本地待办索引无法读取，请保留缓存");
    return Taro.getFileSystemManager().readFileSync(`${Taro.env.USER_DATA_PATH}/${filename}`, "utf8") as string;
  },
  write(key, value) {
    const fs = Taro.getFileSystemManager();
    const previous = Taro.getStorageSync<string>(key);
    const filename = `todo-${key.replace(/[^a-zA-Z0-9_-]/g, "_")}-${newID()}.json`;
    const path = `${Taro.env.USER_DATA_PATH}/${filename}`;
    try {
      fs.writeFileSync(path, value, "utf8");
      Taro.setStorageSync(key, filename);
    } catch (failure) {
      try { fs.unlinkSync(path); } catch { /* No pointer was committed. */ }
      throw failure;
    }
    if (typeof previous === "string" && /^todo-[a-zA-Z0-9_-]+\.json$/.test(previous)) {
      try { fs.unlinkSync(`${Taro.env.USER_DATA_PATH}/${previous}`); } catch { /* Keep committed data if cleanup fails. */ }
    }
  },
};
