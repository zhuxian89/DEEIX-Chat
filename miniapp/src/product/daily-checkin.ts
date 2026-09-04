export type DailyCheckinPrize = {
  prizeKey: string;
  calls: number;
  weightBps: number;
};

export type DailyCheckinWheelSegment = {
  color: string;
  endPercent: number;
  midpointDegrees: number;
  prize: DailyCheckinPrize;
  showLabel: boolean;
  startPercent: number;
};

const WHEEL_COLORS = ["#ff9f43", "#ff6b57", "#8065f5", "#4f86ed", "#2fb795", "#ffd15c"];
const FULL_TURN_DEGREES = 360;
const SPIN_TURNS = 5;
const LABEL_RADIUS_PERCENT = 30;
const MIN_INLINE_LABEL_PERCENT = 9;

export function wheelSegments(prizes: DailyCheckinPrize[]): DailyCheckinWheelSegment[] {
  const totalWeight = prizes.reduce((total, prize) => total + prize.weightBps, 0);
  if (prizes.length === 0 || totalWeight <= 0) {
    return [];
  }

  let cumulativeWeight = 0;
  return prizes.map((prize, index) => {
    const startPercent = cumulativeWeight / totalWeight * 100;
    cumulativeWeight += prize.weightBps;
    const endPercent = cumulativeWeight / totalWeight * 100;
    return {
      color: WHEEL_COLORS[index % WHEEL_COLORS.length],
      endPercent,
      midpointDegrees: (startPercent + endPercent) / 2 / 100 * FULL_TURN_DEGREES,
      prize,
      showLabel: endPercent - startPercent >= MIN_INLINE_LABEL_PERCENT,
      startPercent,
    };
  });
}

export function wheelGradient(prizes: DailyCheckinPrize[]): string {
  const segments = wheelSegments(prizes);
  if (segments.length === 0) {
    return "#f3f0ff";
  }
  const stops = segments.map(
    (segment) => `${segment.color} ${segment.startPercent}% ${segment.endPercent}%`,
  );
  return `conic-gradient(${stops.join(", ")})`;
}

export function wheelLabelPosition(
  segment: DailyCheckinWheelSegment,
): { left: string; top: string } {
  const radians = (segment.midpointDegrees - 90) * Math.PI / 180;
  return {
    left: `${50 + Math.cos(radians) * LABEL_RADIUS_PERCENT}%`,
    top: `${50 + Math.sin(radians) * LABEL_RADIUS_PERCENT}%`,
  };
}

export function wheelRotationForPrize(
  prizes: DailyCheckinPrize[],
  prizeKey: string,
  currentRotation: number,
): number {
  const segment = wheelSegments(prizes).find((item) => item.prize.prizeKey === prizeKey);
  if (!segment) {
    return currentRotation + SPIN_TURNS * FULL_TURN_DEGREES;
  }
  const targetWithinTurn = (FULL_TURN_DEGREES - segment.midpointDegrees) % FULL_TURN_DEGREES;
  const currentTurn = Math.floor(currentRotation / FULL_TURN_DEGREES);
  let target = (currentTurn + SPIN_TURNS) * FULL_TURN_DEGREES + targetWithinTurn;
  if (target <= currentRotation + (SPIN_TURNS - 1) * FULL_TURN_DEGREES) {
    target += FULL_TURN_DEGREES;
  }
  return target;
}
