declare module "*.vue" {
  import type { DefineComponent } from "vue";
  const component: DefineComponent<{}, {}, any>;
  export default component;
}

interface Window {
  __SUB2API__?: {
    controlPort: number;
    controlToken: string;
  };
}
