<?php

namespace FlowCatalyst\Generated\Endpoint;

class PutApiAdminConfigAccessByAppCodeByRoleCode extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $appCode;
    protected $roleCode;
    /**
     * @param string $appCode
     * @param string $roleCode
     * @param null|\FlowCatalyst\Generated\Model\UpdateAccessRequest $requestBody
     */
    public function __construct(string $appCode, string $roleCode, ?\FlowCatalyst\Generated\Model\UpdateAccessRequest $requestBody = null)
    {
        $this->appCode = $appCode;
        $this->roleCode = $roleCode;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'PUT';
    }
    public function getUri(): string
    {
        return str_replace(['{appCode}', '{roleCode}'], [$this->appCode, $this->roleCode], '/api/admin/config-access/{appCode}/{roleCode}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\UpdateAccessRequest) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminConfigAccessByAppCodeByRoleCodeForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminConfigAccessByAppCodeByRoleCodeNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminConfigAccessByAppCodeByRoleCodeBadRequestException
     *
     * @return null
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return json_decode($body);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PutApiAdminConfigAccessByAppCodeByRoleCodeForbiddenException($response);
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PutApiAdminConfigAccessByAppCodeByRoleCodeNotFoundException($response);
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PutApiAdminConfigAccessByAppCodeByRoleCodeBadRequestException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}