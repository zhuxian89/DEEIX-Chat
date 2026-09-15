"use client";

import { ImagePlus } from "lucide-react";
import { usePathname, useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { SidebarMenuItem, SidebarTransitionContent } from "@/components/ui/sidebar";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useChatSession } from "@/features/chat/context/chat-session-context";
import { cn } from "@/lib/utils";

export function NewImageConversation({ isCollapsed, onCloseMobileSidebar }: {
  isCollapsed: boolean;
  onCloseMobileSidebar: () => void;
}) {
  const t = useTranslations("common.navigation");
  const pathname = usePathname();
  const router = useRouter();
  const { requestNewConversation } = useChatSession();
  const title = t("createImage");

  function createImageConversation() {
    requestNewConversation({ imageDefault: true });
    if (pathname === "/chat") {
      window.history.pushState(null, "", "/chat");
    } else {
      router.push("/chat");
    }
    onCloseMobileSidebar();
  }

  return (
    <SidebarMenuItem>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            aria-label={title}
            className={cn(
              "h-8 gap-0 overflow-hidden rounded-md px-0 text-left text-sm font-normal text-sidebar-foreground transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
              isCollapsed ? "w-8 justify-center" : "w-full justify-start",
            )}
            onClick={createImageConversation}
          >
            <span className="flex w-8 shrink-0 items-center justify-center">
              <ImagePlus aria-hidden strokeWidth={1.6} className="size-4" />
            </span>
            <SidebarTransitionContent asChild>
              <span className={cn("ml-1 min-w-0 flex-1 truncate", isCollapsed && "ml-0")}>{title}</span>
            </SidebarTransitionContent>
          </Button>
        </TooltipTrigger>
        <TooltipContent side="right" hidden={!isCollapsed}>{title}</TooltipContent>
      </Tooltip>
    </SidebarMenuItem>
  );
}
