declare namespace JSX {
  interface IntrinsicElements {
    "mp-html": {
      content: string;
      markdown?: boolean;
      selectable?: boolean | "force";
      scrollTable?: boolean;
      copyLink?: boolean;
      previewImg?: boolean;
      showImgMenu?: boolean;
      setTitle?: boolean;
    };
  }
}
