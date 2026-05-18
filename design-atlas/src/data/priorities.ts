export type Priority = "P0" | "P1" | "P2";

export const priorityOrder: Priority[] = ["P0", "P1", "P2"];

export const priorityDisplayLabels: Record<Priority, string> = {
  P0: "第一版生命线",
  P1: "结构化增强",
  P2: "未来演进",
};
