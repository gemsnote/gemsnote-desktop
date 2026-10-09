#import <Cocoa/Cocoa.h>

static BOOL GemsnoteChinese(NSArray<NSString *> *languages) {
    NSString *first = [[languages firstObject] lowercaseString];
    NSString *code = [[first stringByReplacingOccurrencesOfString:@"_" withString:@"-"] componentsSeparatedByString:@"-"].firstObject;
    return [code isEqualToString:@"zh"];
}

static void GemsnoteTranslateMenu(NSMenu *menu, BOOL chinese, NSString *appName) {
    // Match native selectors, not English labels. Targets, responder-chain
    // handling, enabled state and key equivalents remain owned by Wails/AppKit.
    NSDictionary *labels = @{
        @"About": @[@"About %@", @"关于 %@"],
        @"hide:": @[@"Hide %@", @"隐藏 %@"],
        @"hideOtherApplications:": @[@"Hide Others", @"隐藏其他"],
        @"unhideAllApplications:": @[@"Show All", @"全部显示"],
        @"Quit": @[@"Quit %@", @"退出 %@"],
        @"undo:": @[@"Undo", @"撤销"],
        @"redo:": @[@"Redo", @"重做"],
        @"cut:": @[@"Cut", @"剪切"],
        @"copy:": @[@"Copy", @"拷贝"],
        @"paste:": @[@"Paste", @"粘贴"],
        @"pasteAsRichText:": @[@"Paste and Match Style", @"粘贴并匹配样式"],
        @"delete:": @[@"Delete", @"删除"],
        @"selectAll:": @[@"Select All", @"全选"],
        @"startSpeaking:": @[@"Start Speaking", @"开始朗读"],
        @"stopSpeaking:": @[@"Stop Speaking", @"停止朗读"],
        @"performMiniaturize:": @[@"Minimize", @"最小化"],
        @"performZoom:": @[@"Zoom", @"缩放"],
        @"enterFullScreenMode:": @[@"Full Screen", @"进入全屏幕"]
    };
    NSDictionary *submenus = @{
        @"Edit": @"编辑", @"Window": @"窗口", @"Speech": @"语音"
    };
    for (NSMenuItem *item in menu.itemArray) {
        if (item.action != NULL) {
            NSArray *pair = labels[NSStringFromSelector(item.action)];
            if (pair != nil) {
                item.title = [pair[chinese ? 1 : 0] stringByReplacingOccurrencesOfString:@"%@" withString:appName];
            }
        }
        if (item.submenu != nil) {
            for (NSString *english in submenus) {
                if ([item.title isEqualToString:english] || [item.title isEqualToString:submenus[english]]) {
                    item.title = chinese ? submenus[english] : english;
                    item.submenu.title = item.title;
                    break;
                }
            }
            GemsnoteTranslateMenu(item.submenu, chinese, appName);
        }
    }
}

void GemsnoteLocalizeMenu(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            NSString *name = [NSRunningApplication currentApplication].localizedName;
            GemsnoteTranslateMenu(NSApp.mainMenu, GemsnoteChinese([NSLocale preferredLanguages]), name ?: @"Gemsnote");
        }
    });
}
