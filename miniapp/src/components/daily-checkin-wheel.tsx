import type { DailyCheckinStatusResponse } from "@deeix/api-contract";
import { Button, Text, View } from "@tarojs/components";
import { wheelGradient, wheelLabelPosition } from "@/product/daily-checkin";

type DailyCheckinWheelProps = {
  status: DailyCheckinStatusResponse;
  isClaiming: boolean;
  rotation: number;
  revealResult: boolean;
  onClaim(): void;
};

function claimButtonLabel(claiming: boolean, claimed: boolean): string {
  if (claiming) {
    return "好运正在揭晓";
  }
  if (claimed) {
    return "明天再来";
  }
  return "立即签到抽奖";
}

export function DailyCheckinWheel({
  status,
  isClaiming,
  rotation,
  revealResult,
  onClaim,
}: DailyCheckinWheelProps) {
  const highestCalls = Math.max(...status.prizes.map((prize) => prize.calls));
  const claimed = status.claimed && revealResult;

  return (
    <View className="checkinCard">
      <View className="checkinCopy">
        <View>
          <Text className="checkinEyebrow">每日福利</Text>
          <Text className="checkinTitle">每日幸运转盘</Text>
          <Text className="checkinSubtitle">每天免费转，最高 {highestCalls} 次标准对话</Text>
        </View>
        {status.streakDays > 0 ? <Text className="checkinStreak">连续 {status.streakDays} 天</Text> : null}
      </View>

      <View className="checkinBody">
        <View className="wheelStage">
          <View className="wheelPointer" />
          <View
            className={`checkinWheel ${isClaiming ? "checkinWheelSpinning" : ""}`}
            style={{
              background: wheelGradient(status.prizes),
              transform: `rotate(${rotation}deg)`,
            }}
          >
            {status.prizes.map((prize, index) => {
              const position = wheelLabelPosition(status.prizes, index);
              return (
                <View
                  className="wheelLabel"
                  key={prize.prizeKey}
                  style={{
                    left: position.left,
                    top: position.top,
                    transform: `translate(-50%, -50%) rotate(${position.rotation}deg)`,
                  }}
                >
                  <Text className="wheelCalls">{prize.calls}</Text>
                  <Text className="wheelUnit">次</Text>
                </View>
              );
            })}
            <View className="wheelCenter">抽</View>
          </View>
        </View>

        <View className="checkinResult">
          {claimed ? (
            <>
              <Text className="checkinWinLabel">今日好运已到账</Text>
              <Text className="checkinWinValue">{status.awardedCalls} 次</Text>
              <Text className="checkinMoney">折算 ${status.rewardUsd.toFixed(5)}</Text>
            </>
          ) : (
            <>
              <Text className="checkinWinLabel">今天会抽中多少次？</Text>
              <Text className="checkinReady">点击转盘揭晓好运</Text>
            </>
          )}
          <Button
            className="checkinButton"
            disabled={isClaiming || status.claimed}
            loading={isClaiming}
            onClick={onClaim}
          >
            {claimButtonLabel(isClaiming, status.claimed)}
          </Button>
        </View>
      </View>

      <Text className="checkinNotice">
        奖励已存入余额，可用于全部模型；实际可用次数会因所选模型价格不同而变化
      </Text>
    </View>
  );
}
