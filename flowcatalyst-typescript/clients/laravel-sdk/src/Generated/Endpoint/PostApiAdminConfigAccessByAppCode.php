<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostApiAdminConfigAccessByAppCode extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $appCode;
    /**
     * @param string $appCode
     * @param null|\FlowCatalyst\Generated\Model\GrantAccessRequest $requestBody
     */
    public function __construct(string $appCode, ?\FlowCatalyst\Generated\Model\GrantAccessRequest $requestBody = null)
    {
        $this->appCode = $appCode;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return str_replace(['{appCode}'], [$this->appCode], '/api/admin/config-access/{appCode}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\GrantAccessRequest) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminConfigAccessByAppCodeBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminConfigAccessByAppCodeForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminConfigAccessByAppCodeConflictException
     *
     * @return null
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (201 === $status) {
            return null;
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminConfigAccessByAppCodeBadRequestException($response);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminConfigAccessByAppCodeForbiddenException($response);
        }
        if (409 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminConfigAccessByAppCodeConflictException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}