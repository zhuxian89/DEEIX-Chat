/// <reference types="@tarojs/taro" />
/// <reference types="webpack-env" />

declare namespace NodeJS {
  interface ProcessEnv {
    TARO_APP_VALIDATION_MODE?: string;
    TARO_APP_API_BASE_URL?: string;
  }
}
