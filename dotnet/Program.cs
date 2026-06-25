var builder = WebApplication.CreateBuilder(args);

var app = builder.Build();

app.UseHttpsRedirection();

app.Use(async (HttpContext context, RequestDelegate next) =>
{
	var sb = new System.Text.StringBuilder();
	sb.AppendLine($"Method: {context.Request.Method} {context.Request.Protocol}");
	sb.AppendLine($"RequestPath: {context.Request.Path}");
	sb.AppendLine($"RemoteAddr: {context.Connection.RemoteIpAddress}:{context.Connection.RemotePort}");

	foreach (var header in context.Request.Headers)
	{
		sb.AppendLine($"{header.Key}: {header.Value}");
	}

	context.Response.ContentType = "text/plain";
	await context.Response.WriteAsync(sb.ToString());
});

app.Run();
