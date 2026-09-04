export type DailyCheckInPrize = {
  prizeKey: string;
  calls: number;
  weightBps: number;
};

const WHEEL_COLORS = ["#ffb84d", "#ff7657", "#8b6cff", "#5c8dff", "#39b99a", "#ffd45c"];
const FULL_TURN_DEGREES = 360;
const SPIN_TURNS = 5;

export function wheelGradient(prizes: DailyCheckInPrize[]): string {
  if (prizes.length === 0) {
    return "#f3f0ff";
  }
  const segments = prizes.map((_, index) => {
    const start = index / prizes.length * 100;
    const end = (index + 1) / prizes.length * 100;
    return `${WHEEL_COLORS[index % WHEEL_COLORS.length]} ${start}% ${end}%`;
  });
  return `conic-gradient(from -90deg, ${segments.join(", ")})`;
}

export function wheelLabelPosition(
  prizes: DailyCheckInPrize[],
  index: number,
): { left: string; top: string; rotation: number } {
  const angle = (index + 0.5) / prizes.length * FULL_TURN_DEGREES - 90;
  const radians = angle * Math.PI / 180;
  return {
    left: `${50 + Math.cos(radians) * 32}%`,
    top: `${50 + Math.sin(radians) * 32}%`,
    rotation: angle + 90,
  };
}

export function wheelRotationForPrize(
  prizes: DailyCheckInPrize[],
  prizeKey: string,
  currentRotation: number,
): number {
  const index = prizes.findIndex((prize) => prize.prizeKey === prizeKey);
  if (index < 0) {
    return currentRotation + SPIN_TURNS * FULL_TURN_DEGREES;
  }
  const middleDegrees = (index + 0.5) / prizes.length * FULL_TURN_DEGREES;
  const targetWithinTurn = (FULL_TURN_DEGREES - middleDegrees) % FULL_TURN_DEGREES;
  const currentTurn = Math.floor(currentRotation / FULL_TURN_DEGREES);
  let target = (currentTurn + SPIN_TURNS) * FULL_TURN_DEGREES + targetWithinTurn;
  if (target <= currentRotation + (SPIN_TURNS - 1) * FULL_TURN_DEGREES) {
    target += FULL_TURN_DEGREES;
  }
  return target;
}
