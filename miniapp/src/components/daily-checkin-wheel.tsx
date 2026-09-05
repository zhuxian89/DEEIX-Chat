import type { DailyCheckinStatusResponse } from "@deeix/api-contract";
import { Button, Text, View } from "@tarojs/components";
import {
  wheelGradient,
  wheelLabelPosition,
  wheelSegments,
} from "@/product/daily-checkin";

type DailyCheckinEntryProps = {
  status: DailyCheckinStatusResponse;
  onOpen(): void;
};

type DailyCheckinWheelProps = {
  status: DailyCheckinStatusResponse;
  isClaiming: boolean;
  rotation: number;
  revealResult: boolean;
  onClaim(): void;
};

function prizeRange(status: DailyCheckinStatusResponse): { min: number; max: number } {
  const calls = status.prizes.map((prize) => prize.calls);
  return {
    min: calls.length > 0 ? Math.min(...calls) : 0,
    max: calls.length > 0 ? Math.max(...calls) : 0,
  };
}

function claimButtonLabel(claiming: boolean, claimed: boolean): string {
  if (claiming) {
    return "好运正在揭晓";
  }
  if (claimed) {
    return "明天再来";
  }
  return "立即签到抽奖";
}

function formatProbability(weightBps: number): string {
  const percent = weightBps / 100;
  return `${Number.isInteger(percent) ? percent : percent.toFixed(2)}%`;
}

export function DailyCheckinEntry({ status, onOpen }: DailyCheckinEntryProps) {
  const range = prizeRange(status);
  const summary = status.claimed
    ? `今日已增加 ${status.awardedCalls} 次标准对话`
    : `签到可抽 ${range.min}–${range.max} 次标准对话`;

  return (
    <View
      className={`checkinEntry ${status.claimed ? "checkinEntryClaimed" : "checkinEntryPending"}`}
      onClick={onOpen}
    >
      {status.claimed ? null : (
        <>
          <View className="checkinEntryGlow" />
          <View className="checkinEntryShimmer" />
        </>
      )}
      <View className="checkinEntryIcon">
        <Text className="checkinEntryIconText">签</Text>
      </View>
      <View className="checkinEntryCopy">
        <Text className="checkinEntryEyebrow">每日福利</Text>
        <Text className="checkinEntryTitle">{summary}</Text>
      </View>
      <View className="checkinEntryAction">
        {status.claimed ? null : <Text className="checkinEntryActionText">今日可领</Text>}
        <Text className="checkinEntryArrow">›</Text>
      </View>
    </View>
  );
}

export function DailyCheckinWheel({
  status,
  isClaiming,
  rotation,
  revealResult,
  onClaim,
}: DailyCheckinWheelProps) {
  const range = prizeRange(status);
  const segments = wheelSegments(status.prizes);
  const claimed = status.claimed && revealResult;

  return (
    <View className="checkinPanel">
      <View className="checkinPageIntro">
        <Text className="checkinPageEyebrow">每日一次</Text>
        <Text className="checkinPageTitle">转动今天的好运</Text>
        <Text className="checkinPageSubtitle">
          每天可抽 {range.min}–{range.max} 次标准对话奖励
        </Text>
        {status.streakDays > 0 ? (
          <Text className="checkinPageStreak">已连续签到 {status.streakDays} 天</Text>
        ) : null}
      </View>

      <View className="wheelStage">
        <View className="wheelPointer" />
        <View
          className={`checkinWheel ${isClaiming ? "checkinWheelSpinning" : ""}`}
          style={{
            background: wheelGradient(status.prizes),
            transform: `rotate(${rotation}deg)`,
          }}
        >
          {segments.map((segment) => {
            const position = wheelLabelPosition(segment);
            return (
              <View
                className="wheelLabel"
                key={segment.prize.prizeKey}
                style={{ left: position.left, top: position.top }}
              >
                <View
                  className="wheelLabelContent"
                  style={{ transform: `rotate(${-rotation}deg)` }}
                >
                  <Text className="wheelCalls">{segment.prize.calls}</Text>
                  <Text className="wheelUnit">次</Text>
                </View>
              </View>
            );
          })}
        </View>
        <View
          className={`wheelCenter ${status.claimed ? "wheelCenterClaimed" : ""}`}
          onClick={onClaim}
        >
          <Text className="wheelCenterMain">抽</Text>
          <Text className="wheelCenterHint">好运</Text>
        </View>
      </View>

      <View className="checkinPrizeCard">
        <View className="checkinPrizeHeader">
          <Text>奖励档位</Text>
          <Text>中奖机会</Text>
        </View>
        <View className="checkinPrizeGrid">
          {segments.map((segment) => (
            <View className="checkinPrizeItem" key={segment.prize.prizeKey}>
              <View className="checkinPrizeColor" style={{ background: segment.color }} />
              <Text className="checkinPrizeCalls">{segment.prize.calls} 次</Text>
              <Text className="checkinPrizeProbability">
                {formatProbability(segment.prize.weightBps)}
              </Text>
            </View>
          ))}
        </View>
      </View>

      <View className={`checkinResult ${claimed ? "checkinResultClaimed" : ""}`}>
        {claimed ? (
          <>
            <Text className="checkinWinLabel">今日奖励已到账</Text>
            <Text className="checkinWinValue">+{status.awardedCalls} 次</Text>
            <Text className="checkinMoney">已增加余额 ${status.rewardUsd.toFixed(5)}</Text>
          </>
        ) : (
          <>
            <Text className="checkinWinLabel">今天会抽中多少次？</Text>
            <Text className="checkinReady">点击按钮，转盘将为你揭晓</Text>
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

      <Text className="checkinNotice">
        转盘采用等分展示，实际中奖机会以奖励列表为准。奖励按标准对话单价折算后存入余额，可用于全部模型；实际可用次数会因模型价格不同而变化
      </Text>
    </View>
  );
}
