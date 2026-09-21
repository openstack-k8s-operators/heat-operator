# TODO(ramishra): remove fallback once antelope images are no longer tested
try:
    from heat.wsgi.api import application
except ImportError:
    from heat.httpd.heat_api import init_application

    application = init_application()
