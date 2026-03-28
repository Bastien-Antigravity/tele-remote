#!/usr/bin/env python
# coding:utf-8

from os.path import join as osPathJoin
from telebot import TeleBot, types


# relative import
from sys import path;path.extend("..")
from common.Helpers.helpers import TreeNode
from common.Helpers.os_helpers import get_executed_script_dir, host_has_proxy
from common.Helpers.helpers import getUnusedPort, get_global_config

#if teleremote THREAD: method __call__
#if teleremote ASYNC_MODE: method aCall

MAIN_MENU = None
_BACK = None
UNDER_CONSTRUCTION_ = None
all_message_handlers = {}
current_config = None
current_logger = None


def init_bot(config, logger, async_mode=False):
    global current_config ; current_config = config
    global current_logger ; current_logger = logger
    hasProxy, _ = host_has_proxy()
    if hasProxy:
        if async_mode: from telebot.asyncio_helper import proxy as telebotProxy 
        else: from telebot.apihelper import proxy as telebotProxy
        telebotProxy={"http":"http:127.0.0.1:{0}".format(getUnusedPort())}
    if async_mode: 
        from telebot.async_telebot import AsyncTeleBot
        Bot = AsyncTeleBot(config.TB_TOKEN) 
    else: 
        Bot = TeleBot(config.TB_TOKEN, threaded=False)
    return Bot


class BaseMixin(TreeNode):
    def add_sub_menu(self, obj):
        self.add_child(obj)
        obj_markup = types.InlineKeyboardButton(text=obj.caption)
        self.markup.row(obj_markup)
        all_message_handlers[obj.caption] = obj
    def refresh_sub_menu(self, old_caption, new_caption):
        for i, button in enumerate(self.markup.keyboard): 
            if button[0]["text"] == old_caption: 
                button[0]["text"] = new_caption
                break
        all_message_handlers[new_caption] = all_message_handlers.pop(old_caption)
    def update_sub_menu(self, old_obj, new_obj):
        self.drop_sub_menu(obj=old_obj)
        self.add_sub_menu(obj=new_obj)
    def drop_sub_menu(self, obj):
        all_message_handlers.pop(obj.caption)
        for i, button in enumerate(self.markup.keyboard): 
            if button[0]["text"] == obj.caption: 
                self.markup.keyboard.pop(i)
                break
        self.drop_child(obj)
    def update_sub_menu(self, old_obj, new_obj):
        self.drop_sub_menu(old_obj)
        self.add_sub_menu(new_obj)
    def drop_my_child_menu(self, back=None):
        try:
            for obj in self.children:
                all_message_handlers.pop(obj.caption)
        except: pass
        self.drop_my_child()
        self.markup = types.ReplyKeyboardMarkup()
        if not back is None : back = types.InlineKeyboardButton(back.caption)
        else : back = types.InlineKeyboardButton(_BACK.caption)
        self.markup.row(back)


##############################################################################################################################################################################################

##############################################################################################################################################################################################
# standard button
class UNDER_CONSTRUCTION:
    name = 'under construction'
    caption = '🚧 under construction ...'
    tele_message = "🤪 not implemented yet !"
    bot_confirmation = '🚧 '
    starQs_message = 'not implemented yet !'
    markup = None
    def __init__(self):
        pass
    def __call__(self, telecommande, bot):
        bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation)
        bot.send_message(telecommande.config.TB_CHATID, self.tele_message)
    async def aCall(self, telecommande, bot):
        await bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation)
        await bot.send_message(telecommande.config.TB_CHATID, self.tele_message)
    def starQs_response():
        return "not implemented yet !".encode()
UNDER_CONSTRUCTION_ = UNDER_CONSTRUCTION()

class BACK:
    name = 'main menu'
    caption = '📱 main menu'
    bot_confirmation = '📱'
    back_parent_menu = None
    def __init__(self):
        pass
    def __call__(self, telecommande, bot):
        if not self.back_parent_menu is None:
            if self.back_parent_menu.name == 'start':
                self.back_parent_menu(telecommande, bot, FirstCall=False)
            else:
                self.back_parent_menu(telecommande, bot)
        else:
            self.back_parent_menu(telecommande, bot)
    async def aCall(self, telecommande, bot):
        if not self.back_parent_menu is None:
            if self.back_parent_menu.name == 'start':
                await self.back_parent_menu.aCall(telecommande, bot, FirstCall=False)
            else:
                await self.back_parent_menu.aCall(telecommande, bot)
        else:
            await self.back_parent_menu.aCall(telecommande, bot)
_BACK = BACK()

##############################################################################################################################################################################################
# button

class POWER_OFF(BaseMixin):
    name = 'poweroff'
    caption = '🆘 power off !'
    tele_message = None
    bot_confirmation = '🆘'
    starQs_message = 'poweroff'
    markup = None
    def __init__(self):
        TreeNode.__init__(self, node_name=self.caption)     
    def __call__(self, telecommande, bot):
        telecommande.send_msg_in(self.starQs_message)
        bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation)
    async def aCall(self, telecommande, bot):
        await telecommande.send_msg_in(self.starQs_message)
        await bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation)
    def starQs_response(name):
        return "🏛 {0} has been stopped !".format(name)

class CLOSE_ALL_POSITION_CONFIRMATION(BaseMixin):
    name = 'stop'
    caption = '🛑 close all positions, sure ?'
    tele_message = None
    bot_confirmation = '🛑'
    starQs_message = 'stop'
    markup = None
    def __init__(self):
        TreeNode.__init__(self, node_name=self.caption)  
    def __call__(self, telecommande, bot):
        telecommande.send_msg_in(self.starQs_message)
        bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation)
    async def aCall(self, telecommande, bot):
        await telecommande.send_msg_in(self.starQs_message)
        await bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation)
    def starQs_response(name):
        return "🏛 {0} all positions/streams have been closed !".format(name)
    
##############################################################################################################################################################################################
# Menu

class CLOSE_ALL_POSITION(BaseMixin):
    name = 'close all positions'
    caption = '⏏️ close all positions'
    tele_message = None
    bot_confirmation = '⏏️'
    starQs_message = None
    markup = types.ReplyKeyboardMarkup()
    def __init__(self):
        TreeNode.__init__(self, node_name=self.caption) 
        back = types.InlineKeyboardButton(BACK.caption)
        self.markup.row(back) 
        self.add_sub_menu(obj=CLOSE_ALL_POSITION_CONFIRMATION())
    def __call__(self, telecommande, bot):
        bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation, reply_markup=self.markup)
    async def aCall(self, telecommande, bot):
        await bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation, reply_markup=self.markup)

class RUNNING_STRATEGIES(BaseMixin):
    name = 'running strategies'
    caption = '🍀 running strategies.'
    tele_message = None
    bot_confirmation = '🍀'
    starQs_message = None
    markup = types.ReplyKeyboardMarkup()
    def __init__(self):
        TreeNode.__init__(self, node_name=self.caption)
        back = types.InlineKeyboardButton(BACK.caption)
        self.markup.row(back)
    def __call__(self, telecommande, bot):
        bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation, reply_markup=self.markup)
    async def aCall(self, telecommande, bot):
        await bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation, reply_markup=self.markup)

class MENU(BaseMixin):
    name = 'menu'
    caption = '📊 menu...'
    tele_message = None
    bot_confirmation = '📊'
    starQs_message = None
    markup = types.ReplyKeyboardMarkup()
    def __init__(self): 
        TreeNode.__init__(self, node_name=self.caption)
        back = types.InlineKeyboardButton(BACK.caption)
        self.markup.row(back)
    def __call__(self, telecommande, bot):
        bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation, reply_markup=self.markup)
    async def aCall(self, telecommande, bot):
        await bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation, reply_markup=self.markup)

class OPPORTUNITIES(BaseMixin):
    name = 'opportunities'
    caption = '🔭 opportunities :'
    tele_message = None
    bot_confirmation = '🔭'
    starQs_message = None
    markup = types.ReplyKeyboardMarkup()
    def __init__(self):
        TreeNode.__init__(self, node_name=self.caption)
        back = types.InlineKeyboardButton(BACK.caption)
        self.markup.row(back)
    def __call__(self, telecommande, bot):
        bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation, reply_markup=self.markup)
    async def aCall(self, telecommande, bot):
        await bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation, reply_markup=self.markup)

class ARBITRAGE(BaseMixin):
    name = 'arbitrage'
    caption = '📈📉 arbitrage'
    tele_message = 'arbitrage'
    bot_confirmation = '📈📉'
    starQs_message = 'arbitrage'
    markup = types.ReplyKeyboardMarkup()
    sub_menu = None
    def __init__(self):
        TreeNode.__init__(self, node_name=self.caption)
        back = types.InlineKeyboardButton(BACK.caption)
        self.markup.row(back)
        # get arbitre menu : 
        global current_logger
        try:
            from trading.TeleRemote.tele_trading import get_arbitres
            for menu in get_arbitres(all_message_handlers, current_logger):
                self.add_sub_menu(menu)
        except:
            pass
    def __call__(self, telecommande, bot):
        bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation, reply_markup=self.markup)
    async def aCall(self, telecommande, bot):
        await bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation, reply_markup=self.markup)

class CONFIGURATION(BaseMixin):
    name = 'configuration'
    caption = '⚙️ configuration'
    tele_message = 'configuration'
    bot_confirmation = '⚙️'
    starQs_message = 'configuration'
    markup = types.ReplyKeyboardMarkup()
    current_config = None
    def __init__(self):
        from common.config import Config
        from common.TeleRemote.tele_button import init_tele_buttons
        self.current_config = Config(name="MainConfigButton")
        init_tele_buttons(all_message_handlers=all_message_handlers, logger=current_logger)
        TreeNode.__init__(self, node_name=self.caption)
        back = types.InlineKeyboardButton(BACK.caption)
        self.markup.row(back)
    def __call__(self, telecommande, bot):
        self.drop_my_child_menu()
        for button in self.get_config_buttons():
            self.add_sub_menu(button)
        bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation, reply_markup=self.markup)
    async def aCall(self, telecommande, bot):
        self.drop_my_child_menu()
        for button in self.get_config_buttons():
            self.add_sub_menu(button)
        await bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation, reply_markup=self.markup)
    def get_config_buttons(self):
        from common.config import Config
        from common.TeleRemote.tele_button import config_button
        try:
            config_key_val = get_global_config(self.current_config)
        except:
            self.current_config = Config(name="MainConfigButton")
            config_key_val = get_global_config(self.current_config)
        return [config_button(section="{0} {1}".format(self.bot_confirmation, section), item=item, config=self.current_config, bot_confirmation=self.bot_confirmation, parent=self) for section, item in config_key_val.items() if item and len(item)>0]
    

##############################################################################################################################################################################################
class START(BaseMixin):
    name = 'start'
    caption = 'start!'
    tele_message = None
    bot_confirmation = None
    starQs_message = None
    markup = types.ReplyKeyboardMarkup()
    def __init__(self):
        TreeNode.__init__(self, node_name=self.caption)
        self.add_sub_menu(obj=POWER_OFF())
        self.add_sub_menu(obj=CLOSE_ALL_POSITION())
        self.add_sub_menu(obj=RUNNING_STRATEGIES())
        self.add_sub_menu(obj=MENU())
        self.add_sub_menu(obj=OPPORTUNITIES())
        self.add_sub_menu(obj=ARBITRAGE())
        self.add_sub_menu(obj=CONFIGURATION())
    def __call__(self, telecommande, bot, FirstCall=True):
        if FirstCall:
            self.bot_confirmation = self._load_cache()
        else:
            self.bot_confirmation = BACK.bot_confirmation
        if FirstCall:
            bot.send_photo(telecommande.config.TB_CHATID, self.bot_confirmation, reply_markup=self.markup)
        else:
            bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation, reply_markup=self.markup)
    async def aCall(self, telecommande, bot, FirstCall=True):
        if FirstCall:
            self.bot_confirmation = self._load_cache()
        else:
            self.bot_confirmation = BACK.bot_confirmation
        if FirstCall:
            await bot.send_photo(telecommande.config.TB_CHATID, self.bot_confirmation, reply_markup=self.markup)
        else:
            await bot.send_message(telecommande.config.TB_CHATID, self.bot_confirmation, reply_markup=self.markup)

    def _load_cache(self):
        return open(osPathJoin(get_executed_script_dir(__file__), "teleremote_cache", "start.png"), "rb") 

    def Tele_Dispatcher(msg, wait):
        return UNDER_CONSTRUCTION.starQs_response()

###############################################################################################################################################################################################

def create_bot(TeleCommand, Bot, async_mode=False):
    global _BACK; global MAIN_MENU
    if async_mode:
        # Async
        #================================================================    
        # Telecommand interface
        #================================================================
        MAIN_MENU = START()
        _BACK.back_parent_menu = MAIN_MENU
        #================================================================    
        # Message handlers
        #================================================================
        # Handle '/start' 
        @Bot.message_handler(commands=['start'])
        async def send_welcome(message):  
            await MAIN_MENU.aCall(TeleCommand, Bot, FirstCall=True)
        # Handle all received messages
        @Bot.message_handler(func=lambda message: True)
        async def message_handler(message=None):
            try:
                if message.text in all_message_handlers: 
                    await (all_message_handlers[message.text]).aCall(telecommande=TeleCommand, bot=Bot)
                elif message.text == BACK.caption:
                    await _BACK.aCall(TeleCommand, Bot)
                else :
                    await UNDER_CONSTRUCTION_.aCall(TeleCommand, Bot)
            except Exception as e:
                await Bot.send_message(TeleCommand.config.TB_CHATID, "Error while sending command : {0}".format(e))
        # Handle message editing
        @Bot.edited_message_handler(func=lambda message: True)
        async def handle_message_edited(message=None):
            BtnCaption = ''
            try:
                # edit message, config update
                if "√2" in message.text:
                    BtnCaption = message.text.split("√2")[0].strip()
                    if BtnCaption in all_message_handlers:
                        await (all_message_handlers[BtnCaption]).aEdit(telecommande=TeleCommand, bot=Bot, replyMessage=message.text)
                    else :
                        await UNDER_CONSTRUCTION_.aCall(TeleCommand, Bot)
            except Exception as e:
                await Bot.send_message(TeleCommand.config.TB_CHATID, "Error while sending command : {0}".format(e))
        return TeleCommand, Bot
    else:
        # Threaded
        #================================================================    
        # Telecommand interface
        #================================================================
        MAIN_MENU = START()
        _BACK.back_parent_menu = MAIN_MENU#
        #================================================================    
        # Message handlers
        #================================================================
        # Handle '/start' 
        @Bot.message_handler(commands=['start'])
        def send_welcome(message):  
            MAIN_MENU(TeleCommand, Bot, FirstCall=True)
        # Handle all received messages
        @Bot.message_handler(func=lambda message: True)
        def message_handler(message=None):
            try:
                if message.text in all_message_handlers: 
                    (all_message_handlers[message.text])(telecommande=TeleCommand, bot=Bot)
                elif message.text == BACK.caption:
                        _BACK(TeleCommand, Bot)
                else :
                    UNDER_CONSTRUCTION_(TeleCommand, Bot)
            except Exception as e:
                Bot.send_message(TeleCommand.config.TB_CHATID, "Error while sending command : {0}".format(e))#
        # Handle message editing
        @Bot.edited_message_handler(func=lambda message: True)
        def handle_message_edited(message=None):
            BtnCaption = ''
            try:
                # edit message, config update
                if "√2" in message.text:
                    BtnCaption = message.text.split("√2")[0].strip()
                    if BtnCaption in all_message_handlers:
                        (all_message_handlers[BtnCaption]).aEdit(telecommande=TeleCommand, bot=Bot, replyMessage=message.text)
                    else :
                        UNDER_CONSTRUCTION_(TeleCommand, Bot)
            except Exception as e:
                Bot.send_message(TeleCommand.config.TB_CHATID, "Error while sending command : {0}".format(e))
        return TeleCommand, Bot

