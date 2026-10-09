#import "../menu_darwin.m"
#include <assert.h>

int main(void) {
    @autoreleasepool {
        assert(GemsnoteChinese(@[@"zh-Hans-CN"]));
        assert(GemsnoteChinese(@[@"zh-Hant-TW"]));
        assert(GemsnoteChinese(@[@"ZH_HK"]));
        assert(!GemsnoteChinese(@[@"ja-JP", @"zh-CN"]));
        assert(!GemsnoteChinese(@[@"en-US"]));
        assert(!GemsnoteChinese(@[]));
        assert(!GemsnoteChinese(nil));
        [NSApplication sharedApplication];
        NSMenu *root = [[NSMenu alloc] initWithTitle:@""];
        NSMenu *edit = [[NSMenu alloc] initWithTitle:@"Edit"];
        NSMenuItem *top = [[NSMenuItem alloc] initWithTitle:@"Edit" action:NULL keyEquivalent:@""];
        top.submenu = edit;
        [root addItem:top];
        NSObject *target = [[NSObject alloc] init];
        NSMenuItem *copy = [[NSMenuItem alloc] initWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
        copy.target = target;
        copy.keyEquivalentModifierMask = NSEventModifierFlagCommand;
        copy.enabled = NO;
        [edit addItem:copy];
        NSMenuItem *quit = [[NSMenuItem alloc] initWithTitle:@"Quit Gemsnote" action:NSSelectorFromString(@"Quit") keyEquivalent:@"q"];
        quit.target = target;
        [root addItem:quit];
        NSMenuItem *custom = [[NSMenuItem alloc] initWithTitle:@"Custom" action:NSSelectorFromString(@"custom:") keyEquivalent:@""];
        [root addItem:custom];
        GemsnoteTranslateMenu(root, YES, @"珠玑笔记");
        assert([top.title isEqualToString:@"编辑"]);
        assert([edit.title isEqualToString:@"编辑"]);
        assert([copy.title isEqualToString:@"拷贝"]);
        assert([quit.title isEqualToString:@"退出 珠玑笔记"]);
        assert([custom.title isEqualToString:@"Custom"]);
        assert(copy.action == @selector(copy:));
        assert(copy.target == target && quit.target == target);
        assert(!copy.enabled);
        assert([copy.keyEquivalent isEqualToString:@"c"]);
        assert(copy.keyEquivalentModifierMask == NSEventModifierFlagCommand);
        GemsnoteTranslateMenu(root, YES, @"珠玑笔记");
        assert([copy.title isEqualToString:@"拷贝"]);
        GemsnoteTranslateMenu(root, NO, @"Gemsnote");
        assert([top.title isEqualToString:@"Edit"]);
        assert([copy.title isEqualToString:@"Copy"]);
        assert([quit.title isEqualToString:@"Quit Gemsnote"]);
    }
    return 0;
}
